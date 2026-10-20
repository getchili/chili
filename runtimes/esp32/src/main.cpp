/*
 * Firmware Chili pour ESP32.
 *
 * - exécute le programme .chlb stocké en flash (LittleFS), dès le démarrage ;
 * - reçoit de nouveaux programmes par USB série ou par WiFi (TCP, port 7070),
 *   avec le même protocole texte (docs/protocol.md) ;
 * - fournit les natives : print, delay, millis, gpio.*, pwm.*, servo.*, adc.*, i2c.*… (les natives str.* viennent de la VM).
 *
 * La VM rend la main régulièrement (budget d'instructions, delay non
 * bloquant) : les commandes restent traitées même pendant un programme.
 */
#include <Arduino.h>
#include <ESPmDNS.h>
#include <LittleFS.h>
#include <Preferences.h>
#include <WiFi.h>
#include <Wire.h>

#include "chili_natives.h"
#include "chili_vm.h"

#ifndef CHILI_FW_VERSION
#define CHILI_FW_VERSION "dev"
#endif

static const char *PROGRAM_PATH = "/program.chlb";
static const size_t MAX_PROGRAM = 128 * 1024;
static const uint16_t TCP_PORT = 7070;
static const uint32_t STEPS_PER_LOOP = 2000;

/* ------------------------------------------------------------------------ */
/* État                                                                     */

enum ProgState { ST_EMPTY, ST_RUNNING, ST_STOPPED, ST_DONE, ST_ERROR };

static chili_vm vm;  // VM du programme en cours (un programme reçu est vérifié dans une VM temporaire)
static uint8_t *program = nullptr;
static size_t program_size = 0;
static uint32_t program_crc = 0;
static ProgState state = ST_EMPTY;
static uint32_t wake_at = 0;  // fin du delay en cours (millis)
static uint64_t output_pins = 0;  // broches mises en sortie par le programme

static Preferences prefs;
static WiFiServer server(TCP_PORT);
static WiFiClient client;
static bool net_started = false;  // serveur TCP et mDNS lancés
static bool announced = false;    // IP annoncée pour la connexion en cours
static char hostname[16];

struct Channel {
    Stream *io;
    String line;
};
static Channel serial_ch = {&Serial, ""};
static Channel net_ch = {nullptr, ""};

/* Envoie une ligne sur l'USB et sur le client WiFi éventuel. */
static void broadcast(const String &s)
{
    Serial.println(s);
    if (client && client.connected())
        client.println(s);
}

static void reply(Channel &ch, const String &s) { ch.io->println(s); }

/* Envoie des octets bruts (sans fin de ligne) sur l'USB et le client WiFi. */
static void emit(const char *buf, size_t n)
{
    Serial.write((const uint8_t *)buf, n);
    if (client && client.connected())
        client.write((const uint8_t *)buf, n);
}

/* ------------------------------------------------------------------------ */
/* Natives (même ordre que vm/src/chili_natives.h)                          */

static void pwm_detach(int pin);

static chili_status native_print(chili_vm *vm, const chili_value *args, chili_value *)
{
    char buf[64];
    uint32_t len;
    const char *text = chili_text(vm, args[0], buf, sizeof buf, &len);
    emit(text, len);
    emit("\r\n", 2);
    return CHILI_OK;
}

static chili_status native_delay(chili_vm *, const chili_value *args, chili_value *)
{
    if (args[0].v <= 0)
        return CHILI_OK;
    wake_at = millis() + (uint32_t)args[0].v;
    return CHILI_YIELD;  // la boucle principale reprend la VM après wake_at
}

static chili_status native_millis(chili_vm *, const chili_value *, chili_value *ret)
{
    *ret = chili_int((int32_t)millis());
    return CHILI_OK;
}

/*
 * Refuse les broches qu'un programme ne doit pas reconfigurer : 1 et 3 portent
 * le port USB série (les toucher coupe la liaison avec le PC, la carte semble
 * morte), 6 à 11 sont reliées à la mémoire flash (les toucher plante la puce).
 */
static chili_status check_free_pin(chili_vm *vm, const char *who, int32_t pin)
{
    if (pin == 1 || pin == 3)
        return chili_fail(vm, "%s : la broche %ld sert au port USB (%s), l'utiliser couperait la liaison avec le PC",
                          who, (long)pin, pin == 1 ? "TX" : "RX");
    if (pin >= 6 && pin <= 11)
        return chili_fail(vm, "%s : la broche %ld est reliee a la memoire flash, elle n'est pas utilisable", who, (long)pin);
    return CHILI_OK;
}

static chili_status native_gpio_mode(chili_vm *vm, const chili_value *args, chili_value *)
{
    int32_t pin = args[0].v, mode = args[1].v;
    if (pin < 0 || !GPIO_IS_VALID_GPIO(pin))
        return chili_fail(vm, "gpio.mode : broche %ld invalide", (long)pin);
    if (check_free_pin(vm, "gpio.mode", pin) != CHILI_OK)
        return CHILI_ERROR;
    pwm_detach(pin);
    switch (mode) {
    case 0: pinMode(pin, INPUT); break;
    case 1:
        if (!GPIO_IS_VALID_OUTPUT_GPIO(pin))
            return chili_fail(vm, "gpio.mode : la broche %ld ne peut pas etre une sortie", (long)pin);
        pinMode(pin, OUTPUT);
        output_pins |= 1ULL << pin;
        break;
    case 2: pinMode(pin, INPUT_PULLUP); break;
    case 3: pinMode(pin, INPUT_PULLDOWN); break;
    default: return chili_fail(vm, "gpio.mode : mode %ld inconnu (0 a 3)", (long)mode);
    }
    return CHILI_OK;
}

static chili_status native_gpio_write(chili_vm *vm, const chili_value *args, chili_value *)
{
    int32_t pin = args[0].v;
    if (pin < 0 || !GPIO_IS_VALID_OUTPUT_GPIO(pin))
        return chili_fail(vm, "gpio.write : broche %ld invalide", (long)pin);
    if (check_free_pin(vm, "gpio.write", pin) != CHILI_OK)
        return CHILI_ERROR;
    digitalWrite(pin, args[1].v ? HIGH : LOW);
    return CHILI_OK;
}

static chili_status native_gpio_read(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    int32_t pin = args[0].v;
    if (pin < 0 || !GPIO_IS_VALID_GPIO(pin))
        return chili_fail(vm, "gpio.read : broche %ld invalide", (long)pin);
    *ret = chili_int(digitalRead(pin));
    return CHILI_OK;
}

static chili_status native_printc(chili_vm *vm, const chili_value *args, chili_value *)
{
    char buf[4];
    int n = chili_utf8(vm, args[0].v, buf);
    if (n == 0)
        return CHILI_ERROR;
    emit(buf, (size_t)n);
    return CHILI_OK;
}

static chili_status native_println(chili_vm *, const chili_value *, chili_value *)
{
    emit("\r\n", 2);
    return CHILI_OK;
}

static chili_status native_random(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    // Générateur matériel de l'ESP32 : vrai aléatoire, pas de graine à gérer.
    return chili_random_range(vm, args[0].v, args[1].v, esp_random(), &ret->v);
}

/* ------------------------------------------------------------------------ */
/* PWM, son et servo : périphérique LEDC                                    */
/*
 * Les canaux LEDC vont par paires qui partagent le même timer (donc la même
 * fréquence) : on n'utilise que les canaux pairs, soit 8 broches à la fois,
 * chacune avec sa propre fréquence.
 */

enum PwmMode { PWM_NONE, PWM_DUTY, PWM_TONE, PWM_SERVO };
static const int PWM_CHANNELS = 8;
static int8_t pwm_pin[PWM_CHANNELS] = {-1, -1, -1, -1, -1, -1, -1, -1};
static uint8_t pwm_mode[PWM_CHANNELS];

/* Canal LEDC de la broche, configuré pour le mode demandé ; -1 si plus de canal libre. */
static int pwm_channel(int pin, PwmMode mode)
{
    int slot = -1;
    for (int i = 0; i < PWM_CHANNELS; i++) {
        if (pwm_pin[i] == pin) {
            slot = i;
            break;
        }
        if (slot < 0 && pwm_pin[i] < 0)
            slot = i;
    }
    if (slot < 0)
        return -1;
    int ch = slot * 2;
    if (pwm_pin[slot] != pin || pwm_mode[slot] != mode) {
        switch (mode) {
        case PWM_SERVO: ledcSetup(ch, 50, 16); break;   // 50 Hz, 16 bits : impulsions précises
        default:        ledcSetup(ch, 5000, 8); break;  // 5 kHz, 8 bits (le son règle sa fréquence)
        }
        if (pwm_pin[slot] != pin)
            ledcAttachPin(pin, ch);
        pwm_pin[slot] = (int8_t)pin;
        pwm_mode[slot] = mode;
    }
    return ch;
}

/* Rend une broche au GPIO normal si elle était en PWM (appelé par gpio.mode). */
static void pwm_detach(int pin)
{
    for (int i = 0; i < PWM_CHANNELS; i++) {
        if (pwm_pin[i] == pin) {
            ledcWrite(i * 2, 0);
            ledcDetachPin(pin);
            pwm_pin[i] = -1;
        }
    }
}

/* Arrête toutes les sorties PWM (fin ou arrêt du programme). */
static void pwm_release()
{
    for (int i = 0; i < PWM_CHANNELS; i++) {
        if (pwm_pin[i] >= 0) {
            ledcWrite(i * 2, 0);
            ledcDetachPin(pwm_pin[i]);
            pinMode(pwm_pin[i], INPUT);
            pwm_pin[i] = -1;
        }
    }
}

static chili_status pwm_check_pin(chili_vm *vm, const char *who, int32_t pin, int *ch, PwmMode mode)
{
    if (pin < 0 || !GPIO_IS_VALID_OUTPUT_GPIO(pin))
        return chili_fail(vm, "%s : la broche %ld ne peut pas etre une sortie", who, (long)pin);
    if (check_free_pin(vm, who, pin) != CHILI_OK)
        return CHILI_ERROR;
    *ch = pwm_channel(pin, mode);
    if (*ch < 0)
        return chili_fail(vm, "%s : deja %d broches en PWM, c'est le maximum", who, PWM_CHANNELS);
    return CHILI_OK;
}

static chili_status native_pwm_write(chili_vm *vm, const chili_value *args, chili_value *)
{
    int ch;
    if (chili_check_range(vm, "pwm.write", "valeur", args[1].v, 0, CHILI_PWM_MAX) != CHILI_OK ||
        pwm_check_pin(vm, "pwm.write", args[0].v, &ch, PWM_DUTY) != CHILI_OK)
        return CHILI_ERROR;
    ledcWrite(ch, (uint32_t)args[1].v);
    return CHILI_OK;
}

static chili_status native_pwm_tone(chili_vm *vm, const chili_value *args, chili_value *)
{
    int32_t hz = args[1].v;
    int ch;
    if (hz != 0 && (hz < 20 || hz > CHILI_TONE_MAX))
        return chili_fail(vm, "pwm.tone : frequence %ld Hz hors de 20..%d (0 pour couper le son)",
                          (long)hz, CHILI_TONE_MAX);
    if (pwm_check_pin(vm, "pwm.tone", args[0].v, &ch, PWM_TONE) != CHILI_OK)
        return CHILI_ERROR;
    if (hz == 0)
        ledcWrite(ch, 0);
    else
        ledcWriteTone(ch, (uint32_t)hz);  // signal carré, rapport cyclique 50 %
    return CHILI_OK;
}

static chili_status native_servo_write(chili_vm *vm, const chili_value *args, chili_value *)
{
    int ch;
    if (chili_check_range(vm, "servo.write", "angle", args[1].v, 0, CHILI_SERVO_MAX) != CHILI_OK ||
        pwm_check_pin(vm, "servo.write", args[0].v, &ch, PWM_SERVO) != CHILI_OK)
        return CHILI_ERROR;
    // Période de 20 ms sur 65536 pas : impulsion de 500 à 2500 µs.
    uint32_t duty = (uint32_t)CHILI_SERVO_PULSE(args[1].v) * 65536u / 20000u;
    ledcWrite(ch, duty);
    return CHILI_OK;
}

/* ------------------------------------------------------------------------ */
/* Entrées analogiques                                                      */

/* Vérifie qu'une broche a une entrée analogique utilisable. */
static chili_status adc_check_pin(chili_vm *vm, const char *who, int32_t pin)
{
    int8_t ch = (pin >= 0 && pin < 40) ? digitalPinToAnalogChannel(pin) : -1;
    if (ch < 0)
        return chili_fail(vm, "%s : la broche %ld n'a pas d'entree analogique (utilise 32 a 39)", who, (long)pin);
    // Les canaux 10 et plus (ADC2) sont pris par le WiFi quand il est actif.
    if (ch >= 10 && WiFi.getMode() != WIFI_OFF)
        return chili_fail(vm, "%s : la broche %ld ne marche pas avec le WiFi actif (utilise 32 a 39)", who, (long)pin);
    return CHILI_OK;
}

static chili_status native_adc_read(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    if (adc_check_pin(vm, "adc.read", args[0].v) != CHILI_OK)
        return CHILI_ERROR;
    *ret = chili_int(analogRead(args[0].v));
    return CHILI_OK;
}

static chili_status native_adc_mv(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    if (adc_check_pin(vm, "adc.mv", args[0].v) != CHILI_OK)
        return CHILI_ERROR;
    *ret = chili_int((int32_t)analogReadMilliVolts(args[0].v));
    return CHILI_OK;
}

/* ------------------------------------------------------------------------ */
/* Bus I2C                                                                  */

static bool i2c_started = false;

/* Démarre le bus sur les broches par défaut (SDA 21, SCL 22) s'il ne l'est pas. */
static void i2c_ensure()
{
    if (!i2c_started) {
        Wire.begin(21, 22, 100000);
        Wire.setTimeOut(50);
        i2c_started = true;
    }
}

static chili_status native_i2c_begin(chili_vm *vm, const chili_value *args, chili_value *)
{
    for (int i = 0; i < 2; i++)
        if (args[i].v < 0 || !GPIO_IS_VALID_OUTPUT_GPIO(args[i].v) ||
            check_free_pin(vm, "i2c.begin", args[i].v) != CHILI_OK)
            return chili_fail(vm, "i2c.begin : la broche %ld ne peut pas servir au bus I2C", (long)args[i].v);
    if (i2c_started)
        Wire.end();
    Wire.begin(args[0].v, args[1].v, 100000);
    Wire.setTimeOut(50);
    i2c_started = true;
    return CHILI_OK;
}

static chili_status native_i2c_scan(chili_vm *vm, const chili_value *, chili_value *ret)
{
    uint8_t found[128];
    uint32_t n = 0;
    i2c_ensure();
    for (uint8_t adr = 1; adr < 127; adr++) {
        Wire.beginTransmission(adr);
        if (Wire.endTransmission() == 0)
            found[n++] = adr;
    }
    if (chili_new_arr(vm, n, ret) != CHILI_OK)
        return CHILI_ERROR;
    for (uint32_t i = 0; i < n; i++)
        chili_arr_set(vm, *ret, i, chili_int(found[i]));
    return CHILI_OK;
}

static chili_status native_i2c_write(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    uint8_t buf[CHILI_I2C_MAX_BYTES];
    uint32_t n;
    if (chili_check_range(vm, "i2c.write", "adresse", args[0].v, 0, 127) != CHILI_OK ||
        chili_bytes(vm, "i2c.write", args[1], buf, sizeof buf, &n) != CHILI_OK)
        return CHILI_ERROR;
    i2c_ensure();
    Wire.beginTransmission((uint8_t)args[0].v);
    Wire.write(buf, n);
    *ret = chili_int(Wire.endTransmission() == 0);  // 1 si le périphérique a répondu
    return CHILI_OK;
}

static chili_status native_i2c_read(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    uint8_t buf[CHILI_I2C_MAX_BYTES];
    uint32_t n = 0;
    if (chili_check_range(vm, "i2c.read", "adresse", args[0].v, 0, 127) != CHILI_OK ||
        chili_check_range(vm, "i2c.read", "taille", args[1].v, 0, CHILI_I2C_MAX_BYTES) != CHILI_OK)
        return CHILI_ERROR;
    i2c_ensure();
    if (args[1].v > 0) {
        Wire.requestFrom((uint8_t)args[0].v, (size_t)args[1].v);
        while (Wire.available() && n < (uint32_t)args[1].v)
            buf[n++] = (uint8_t)Wire.read();
    }
    // Moins d'octets que demandé (ou aucun) : le périphérique n'a pas tout envoyé.
    if (chili_new_arr(vm, n, ret) != CHILI_OK)
        return CHILI_ERROR;
    for (uint32_t i = 0; i < n; i++)
        chili_arr_set(vm, *ret, i, chili_int(buf[i]));
    return CHILI_OK;
}

#define X(id, name, arity, sig, fn) {name, arity, sig, fn},
static const chili_native natives[] = {CHILI_NATIVES(X)};
#undef X

/* ------------------------------------------------------------------------ */
/* Programme                                                                */

/* Remet au repos (niveau bas, puis entrée) les broches que le programme pilotait. */
static void release_outputs()
{
    pwm_release();
    for (int pin = 0; pin < 64; pin++) {
        if (output_pins & (1ULL << pin)) {
            digitalWrite(pin, LOW);
            pinMode(pin, INPUT);
        }
    }
    output_pins = 0;
}

static void start_program()
{
    release_outputs();
    if (!program) {
        state = ST_EMPTY;
        return;
    }
    if (chili_load(&vm, program, program_size, natives, CHILI_NUM_NATIVES, nullptr) != CHILI_OK) {
        state = ST_ERROR;
        broadcast(String("!chili error ") + vm.error);
        return;
    }
    wake_at = millis();
    state = ST_RUNNING;
}

/* Identifiant d'un programme : le CRC de son contenu, stocké dans les 4 derniers
 * octets du .chlb (le CRC du fichier entier, lui, est toujours le même). */
static uint32_t embedded_crc(const uint8_t *buf, size_t n)
{
    if (n < 4)
        return 0;
    const uint8_t *p = buf + n - 4;
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

static void load_stored_program()
{
    File f = LittleFS.open(PROGRAM_PATH, "r");
    if (!f)
        return;
    size_t n = f.size();
    uint8_t *buf = (n > 0 && n <= MAX_PROGRAM) ? (uint8_t *)malloc(n) : nullptr;
    if (buf && f.read(buf, n) == n) {
        program = buf;
        program_size = n;
        program_crc = embedded_crc(buf, n);
    } else {
        free(buf);
    }
    f.close();
}

/* Reçoit `size` octets bruts sur le canal, les vérifie, les enregistre et lance le programme. */
static void cmd_load(Channel &ch, size_t size, uint32_t crc)
{
    if (size == 0 || size > MAX_PROGRAM) {
        reply(ch, "!chili err taille invalide (max " + String(MAX_PROGRAM) + " octets)");
        return;
    }
    uint8_t *buf = (uint8_t *)malloc(size);
    if (!buf) {
        reply(ch, "!chili err memoire insuffisante");
        return;
    }
    reply(ch, "!chili ready");

    size_t got = 0;
    uint32_t last = millis();
    while (got < size && millis() - last < 5000) {
        int avail = ch.io->available();
        if (avail > 0) {
            size_t want = min((size_t)avail, size - got);
            got += ch.io->readBytes(buf + got, want);
            last = millis();
        } else {
            delay(1);
        }
    }
    if (got < size) {
        free(buf);
        reply(ch, "!chili err transfert incomplet (" + String(got) + "/" + String(size) + " octets)");
        return;
    }
    if (chili_crc32(buf, size) != crc) {
        free(buf);
        reply(ch, "!chili err CRC invalide : transfert corrompu");
        return;
    }
    chili_vm *scratch = (chili_vm *)malloc(sizeof(chili_vm));
    if (!scratch) {
        free(buf);
        reply(ch, "!chili err memoire insuffisante pour verifier le programme");
        return;
    }
    bool valid = chili_load(scratch, buf, size, natives, CHILI_NUM_NATIVES, nullptr) == CHILI_OK;
    String why = scratch->error;
    free(scratch);
    if (!valid) {
        free(buf);
        reply(ch, "!chili err programme refuse : " + why);
        return;
    }
    File f = LittleFS.open(PROGRAM_PATH, "w");
    size_t written = f ? f.write(buf, size) : 0;
    if (f)
        f.close();
    if (written != size) {
        free(buf);
        reply(ch, "!chili err ecriture en flash impossible");
        return;
    }

    free(program);
    program = buf;
    program_size = size;
    program_crc = embedded_crc(buf, size);
    prefs.putBool("autorun", true);
    start_program();
    reply(ch, "!chili ok");
}

/* ------------------------------------------------------------------------ */
/* WiFi                                                                     */

static String hex_decode(const String &h)
{
    String out;
    if (h == "-")
        return out;
    for (unsigned i = 0; i + 1 < h.length(); i += 2)
        out += (char)strtol(h.substring(i, i + 2).c_str(), nullptr, 16);
    return out;
}

static void wifi_connect()
{
    announced = false;
    String ssid = prefs.getString("ssid", "");
    if (ssid.isEmpty()) {
        WiFi.disconnect(true);
        WiFi.mode(WIFI_OFF);
        return;
    }
    WiFi.mode(WIFI_STA);
    WiFi.setHostname(hostname);
    WiFi.setAutoReconnect(true);
    WiFi.begin(ssid.c_str(), prefs.getString("pass", "").c_str());
}

static void cmd_wifi(Channel &ch, const String &args)
{
    if (args == "off") {
        prefs.remove("ssid");
        prefs.remove("pass");
        wifi_connect();
        reply(ch, "!chili ok");
        return;
    }
    int sp = args.indexOf(' ');
    if (sp <= 0) {
        reply(ch, "!chili err usage : wifi <ssid hex> <mot de passe hex>|off");
        return;
    }
    prefs.putString("ssid", hex_decode(args.substring(0, sp)));
    prefs.putString("pass", hex_decode(args.substring(sp + 1)));
    wifi_connect();
    reply(ch, "!chili ok");
}

static void poll_network()
{
    if (WiFi.status() != WL_CONNECTED)
        return;
    if (!net_started) {
        server.begin();
        if (MDNS.begin(hostname))
            MDNS.addService("chili", "tcp", TCP_PORT);
        net_started = true;
    }
    if (!announced) {
        announced = true;
        broadcast("!chili wifi ip=" + WiFi.localIP().toString() + " host=" + hostname + ".local");
    }
    if (server.hasClient()) {
        if (client)
            client.stop();
        client = server.available();
        net_ch.io = &client;
        net_ch.line = "";
    }
}

/* ------------------------------------------------------------------------ */
/* Commandes                                                                */

static const char *state_name()
{
    switch (state) {
    case ST_RUNNING: return "running";
    case ST_STOPPED: return "stopped";
    case ST_DONE: return "done";
    case ST_ERROR: return "error";
    default: return "empty";
    }
}

static void cmd_info(Channel &ch)
{
    String chip = ESP.getChipModel();
    chip.replace(' ', '-');
    String s = "!chili info version=" CHILI_FW_VERSION;
    s += " bytecode=" + String(CHILI_BYTECODE_VERSION);
    s += " chip=" + chip;
    s += " heap=" + String(ESP.getFreeHeap());
    if (program) {
        char crc[9];
        snprintf(crc, sizeof crc, "%08lx", (unsigned long)program_crc);
        s += " program=" + String(program_size) + ":" + crc;
    } else {
        s += " program=none";
    }
    s += String(" state=") + state_name();
    s += String(" autorun=") + (prefs.getBool("autorun", true) ? "1" : "0");
    if (WiFi.status() == WL_CONNECTED)
        s += " wifi=connected ip=" + WiFi.localIP().toString() + " host=" + hostname + ".local";
    else if (prefs.getString("ssid", "").length())
        s += " wifi=connecting";
    else
        s += " wifi=off";
    reply(ch, s);
}

static void handle_command(Channel &ch, String cmd)
{
    cmd.trim();
    if (cmd == "info") {
        cmd_info(ch);
    } else if (cmd.startsWith("load ")) {
        int sp = cmd.indexOf(' ', 5);
        size_t size = strtoul(cmd.substring(5, sp).c_str(), nullptr, 10);
        uint32_t crc = strtoul(cmd.substring(sp + 1).c_str(), nullptr, 16);
        cmd_load(ch, size, crc);
    } else if (cmd == "start") {
        if (!program) {
            reply(ch, "!chili err aucun programme");
            return;
        }
        prefs.putBool("autorun", true);
        start_program();
        reply(ch, "!chili ok");
    } else if (cmd == "stop" || cmd == "kill") {
        // stop : pause, le programme repart au prochain démarrage.
        // kill : arrêt durable, plus de lancement au démarrage jusqu'à start/load.
        if (state == ST_RUNNING)
            state = ST_STOPPED;
        release_outputs();
        if (cmd == "kill")
            prefs.putBool("autorun", false);
        reply(ch, "!chili ok");
    } else if (cmd == "erase") {
        release_outputs();
        LittleFS.remove(PROGRAM_PATH);
        free(program);
        program = nullptr;
        program_size = 0;
        state = ST_EMPTY;
        reply(ch, "!chili ok");
    } else if (cmd.startsWith("wifi ")) {
        cmd_wifi(ch, cmd.substring(5));
    } else {
        reply(ch, "!chili err commande inconnue : " + cmd);
    }
}

/* Lit les octets disponibles ; traite chaque ligne `!chili …` complète. */
static void poll_channel(Channel &ch)
{
    if (!ch.io)
        return;
    while (ch.io->available()) {
        char c = ch.io->read();
        if (c == '\n') {
            String line = ch.line;
            ch.line = "";
            line.trim();
            if (line.startsWith("!chili "))
                handle_command(ch, line.substring(7));
        } else if (ch.line.length() < 256) {
            ch.line += c;
        }
    }
}

/* ------------------------------------------------------------------------ */

void setup()
{
    Serial.setRxBufferSize(8192);
    Serial.begin(115200);

    uint64_t mac = ESP.getEfuseMac();
    snprintf(hostname, sizeof hostname, "chili-%02x%02x%02x",
             (unsigned)((mac >> 24) & 0xff), (unsigned)((mac >> 32) & 0xff), (unsigned)((mac >> 40) & 0xff));

    if (!LittleFS.begin(true))
        broadcast("!chili error systeme de fichiers indisponible");
    prefs.begin("chili", false);
    wifi_connect();

    broadcast("!chili boot version=" CHILI_FW_VERSION);
    load_stored_program();
    if (prefs.getBool("autorun", true))
        start_program();
    else if (program)
        state = ST_STOPPED;  // arrêté par chili kill : on attend chili start
}

void loop()
{
    poll_channel(serial_ch);
    poll_network();
    if (client && client.connected())
        poll_channel(net_ch);

    if (state == ST_RUNNING && (int32_t)(millis() - wake_at) >= 0) {
        chili_status st = chili_run(&vm, STEPS_PER_LOOP);
        if (st == CHILI_HALTED) {
            state = ST_DONE;
            broadcast("!chili halt");
        } else if (st == CHILI_ERROR) {
            state = ST_ERROR;
            broadcast(String("!chili error ") + vm.error);
        }
    }
}

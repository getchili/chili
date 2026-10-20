/*
 * Firmware Chili pour ESP32.
 *
 * - exécute le programme .chlb stocké en flash (LittleFS), dès le démarrage ;
 * - reçoit de nouveaux programmes par USB série ou par WiFi (TCP, port 7070),
 *   avec le même protocole texte (docs/protocol.md) ;
 * - fournit les natives : print, delay, millis, gpio.*.
 *
 * La VM rend la main régulièrement (budget d'instructions, delay non
 * bloquant) : les commandes restent traitées même pendant un programme.
 */
#include <Arduino.h>
#include <ESPmDNS.h>
#include <LittleFS.h>
#include <Preferences.h>
#include <WiFi.h>

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

static chili_vm vm;       // VM du programme en cours
static chili_vm scratch;  // sert à vérifier un programme reçu avant de l'accepter
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

/* ------------------------------------------------------------------------ */
/* Natives (même ordre que vm/src/chili_natives.h)                          */

static chili_status native_print(chili_vm *, const int32_t *args, int32_t *)
{
    broadcast(String(args[0]));
    return CHILI_OK;
}

static chili_status native_delay(chili_vm *, const int32_t *args, int32_t *)
{
    if (args[0] <= 0)
        return CHILI_OK;
    wake_at = millis() + (uint32_t)args[0];
    return CHILI_YIELD;  // la boucle principale reprend la VM après wake_at
}

static chili_status native_millis(chili_vm *, const int32_t *, int32_t *ret)
{
    *ret = (int32_t)millis();
    return CHILI_OK;
}

static chili_status native_gpio_mode(chili_vm *vm, const int32_t *args, int32_t *)
{
    int32_t pin = args[0], mode = args[1];
    if (pin < 0 || !GPIO_IS_VALID_GPIO(pin))
        return chili_fail(vm, "gpio.mode : broche %ld invalide", (long)pin);
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

static chili_status native_gpio_write(chili_vm *vm, const int32_t *args, int32_t *)
{
    int32_t pin = args[0];
    if (pin < 0 || !GPIO_IS_VALID_OUTPUT_GPIO(pin))
        return chili_fail(vm, "gpio.write : broche %ld invalide", (long)pin);
    digitalWrite(pin, args[1] ? HIGH : LOW);
    return CHILI_OK;
}

static chili_status native_gpio_read(chili_vm *vm, const int32_t *args, int32_t *ret)
{
    int32_t pin = args[0];
    if (pin < 0 || !GPIO_IS_VALID_GPIO(pin))
        return chili_fail(vm, "gpio.read : broche %ld invalide", (long)pin);
    *ret = digitalRead(pin);
    return CHILI_OK;
}

static chili_status native_random(chili_vm *vm, const int32_t *args, int32_t *ret)
{
    // Générateur matériel de l'ESP32 : vrai aléatoire, pas de graine à gérer.
    return chili_random_range(vm, args[0], args[1], esp_random(), ret);
}

#define X(id, name, arity, fn) {name, arity, fn},
static const chili_native natives[] = {CHILI_NATIVES(X)};
#undef X

/* ------------------------------------------------------------------------ */
/* Programme                                                                */

/* Remet au repos (niveau bas, puis entrée) les broches que le programme pilotait. */
static void release_outputs()
{
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
    if (chili_load(&scratch, buf, size, natives, CHILI_NUM_NATIVES, nullptr) != CHILI_OK) {
        free(buf);
        reply(ch, String("!chili err programme refuse : ") + scratch.error);
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

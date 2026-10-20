/*
 * chili-vm — exécute un fichier .chlb sur PC avec la VM C.
 *
 * Sert à tester la VM contre la VM de référence Go (même sortie attendue que
 * `chili run`). GPIO simulés, delay réel.
 *
 *   chili-vm programme.chlb
 */
#include <stdio.h>
#include <stdlib.h>
#include <time.h>

#include "chili_natives.h"
#include "chili_vm.h"

#ifdef _WIN32
#include <windows.h>
static void sleep_ms(int32_t ms) { Sleep((DWORD)ms); }
static int64_t now_ms(void) { return (int64_t)GetTickCount64(); }
#else
static void sleep_ms(int32_t ms)
{
    struct timespec ts = { ms / 1000, (long)(ms % 1000) * 1000000L };
    nanosleep(&ts, NULL);
}
static int64_t now_ms(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (int64_t)ts.tv_sec * 1000 + ts.tv_nsec / 1000000;
}
#endif

static int64_t start_ms;
static int32_t pins[64];

static chili_status native_print(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    char buf[64];
    uint32_t len;
    const char *text = chili_text(vm, args[0], buf, sizeof buf, &len);
    (void)ret;
    fwrite(text, 1, len, stdout);
    putchar('\n');
    return CHILI_OK;
}

static chili_status native_delay(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)vm; (void)ret;
    if (args[0].v > 0) {
        fflush(stdout);
        sleep_ms(args[0].v);
    }
    return CHILI_OK;
}

static chili_status native_millis(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)vm; (void)args;
    *ret = chili_int((int32_t)(now_ms() - start_ms));
    return CHILI_OK;
}

static chili_status native_gpio_mode(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)vm; (void)ret;
    printf("[gpio] broche %ld en mode %ld\n", (long)args[0].v, (long)args[1].v);
    return CHILI_OK;
}

static chili_status native_gpio_write(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)ret;
    if (args[0].v < 0 || args[0].v >= 64)
        return chili_fail(vm, "broche %ld invalide", (long)args[0].v);
    pins[args[0].v] = args[1].v;
    printf("[gpio] broche %ld = %ld\n", (long)args[0].v, (long)args[1].v);
    return CHILI_OK;
}

static chili_status native_gpio_read(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    if (args[0].v < 0 || args[0].v >= 64)
        return chili_fail(vm, "broche %ld invalide", (long)args[0].v);
    *ret = chili_int(pins[args[0].v]);
    return CHILI_OK;
}

static chili_status native_random(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    /* Deux tirages de rand() (souvent limité à 15 bits) pour couvrir 32 bits. */
    uint32_t r = ((uint32_t)rand() << 17) ^ ((uint32_t)rand() << 2) ^ (uint32_t)rand();
    return chili_random_range(vm, args[0].v, args[1].v, r, &ret->v);
}

static chili_status native_printc(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    char buf[4];
    int n = chili_utf8(vm, args[0].v, buf);
    (void)ret;
    if (n == 0)
        return CHILI_ERROR;
    fwrite(buf, 1, (size_t)n, stdout);
    return CHILI_OK;
}

static chili_status native_println(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)vm; (void)args; (void)ret;
    putchar('\n');
    return CHILI_OK;
}

/* Natives matériel simulées (mêmes messages que chili run, sim.go). */

/* Entrée analogique : rampe de 0 à 4095 puis retour, en 4 secondes. */
static int32_t sim_adc(void)
{
    int64_t t = (now_ms() - start_ms) % 4000;
    if (t > 2000)
        t = 4000 - t;
    return (int32_t)(t * 4095 / 2000);
}

static chili_status native_pwm_write(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)ret;
    if (chili_check_range(vm, "pwm.write", "valeur", args[1].v, 0, CHILI_PWM_MAX) != CHILI_OK)
        return CHILI_ERROR;
    printf("[pwm] broche %ld = %ld/255\n", (long)args[0].v, (long)args[1].v);
    return CHILI_OK;
}

static chili_status native_pwm_tone(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    int32_t hz = args[1].v;
    (void)ret;
    if (hz != 0 && (hz < 20 || hz > CHILI_TONE_MAX))
        return chili_fail(vm, "pwm.tone : frequence %ld Hz hors de 20..%d (0 pour couper le son)",
                          (long)hz, CHILI_TONE_MAX);
    if (hz == 0)
        printf("[pwm] broche %ld : silence\n", (long)args[0].v);
    else
        printf("[pwm] broche %ld : son %ld Hz\n", (long)args[0].v, (long)hz);
    return CHILI_OK;
}

static chili_status native_servo_write(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)ret;
    if (chili_check_range(vm, "servo.write", "angle", args[1].v, 0, CHILI_SERVO_MAX) != CHILI_OK)
        return CHILI_ERROR;
    printf("[servo] broche %ld = %ld deg (impulsion %ld us)\n", (long)args[0].v, (long)args[1].v,
           (long)CHILI_SERVO_PULSE(args[1].v));
    return CHILI_OK;
}

static chili_status native_adc_read(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)vm; (void)args;
    *ret = chili_int(sim_adc());
    return CHILI_OK;
}

static chili_status native_adc_mv(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)vm; (void)args;
    *ret = chili_int(sim_adc() * 3300 / 4095);
    return CHILI_OK;
}

/* Pas de bus I2C sur le PC : aucun périphérique ne répond. */
static chili_status native_i2c_begin(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)vm; (void)ret;
    printf("[i2c] bus sur SDA %ld, SCL %ld\n", (long)args[0].v, (long)args[1].v);
    return CHILI_OK;
}

static chili_status native_i2c_scan(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    (void)args;
    printf("[i2c] scan : aucun peripherique (simulation)\n");
    return chili_new_arr(vm, 0, ret);
}

static chili_status native_i2c_write(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    uint8_t buf[CHILI_I2C_MAX_BYTES];
    uint32_t n;
    (void)ret;
    if (chili_check_range(vm, "i2c.write", "adresse", args[0].v, 0, 127) != CHILI_OK ||
        chili_bytes(vm, "i2c.write", args[1], buf, sizeof buf, &n) != CHILI_OK)
        return CHILI_ERROR;
    printf("[i2c] ecriture de %lu octet(s) vers 0x%02lX (pas de reponse)\n", (unsigned long)n, (unsigned long)args[0].v);
    return CHILI_OK;
}

static chili_status native_i2c_read(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    if (chili_check_range(vm, "i2c.read", "adresse", args[0].v, 0, 127) != CHILI_OK ||
        chili_check_range(vm, "i2c.read", "taille", args[1].v, 0, CHILI_I2C_MAX_BYTES) != CHILI_OK)
        return CHILI_ERROR;
    printf("[i2c] lecture de %ld octet(s) depuis 0x%02lX (pas de reponse)\n", (long)args[1].v, (unsigned long)args[0].v);
    return chili_new_arr(vm, 0, ret);
}

#define X(id, name, arity, sig, fn) { name, arity, sig, fn },
static const chili_native natives[] = { CHILI_NATIVES(X) };
#undef X

static chili_vm vm; /* ~55 Ko : statique plutôt que sur la pile C */

int main(int argc, char **argv)
{
    if (argc != 2) {
        fprintf(stderr, "usage : chili-vm programme.chlb\n");
        return 2;
    }
    FILE *f = fopen(argv[1], "rb");
    if (!f) {
        perror(argv[1]);
        return 1;
    }
    static uint8_t data[1 << 20];
    size_t size = fread(data, 1, sizeof data, f);
    fclose(f);

    if (chili_load(&vm, data, size, natives, CHILI_NUM_NATIVES, NULL) != CHILI_OK) {
        fprintf(stderr, "%s : %s\n", argv[1], vm.error);
        return 1;
    }
    start_ms = now_ms();
    srand((unsigned)time(NULL) ^ (unsigned)start_ms);

    chili_status st;
    do {
        st = chili_run(&vm, 0);
    } while (st == CHILI_BUDGET || st == CHILI_YIELD);

    fflush(stdout);
    if (st == CHILI_ERROR) {
        fprintf(stderr, "erreur d'execution : %s\n", vm.error);
        return 1;
    }
    return 0;
}

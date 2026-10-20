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

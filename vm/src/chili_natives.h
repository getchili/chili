/*
 * chili_natives.h — table des natives, côté C.
 *
 * DOIT suivre exactement compiler/natives/natives.go (même ordre = même ID,
 * même arité). Un test Go (natives_sync_test.go) vérifie cette concordance.
 *
 * Chaque plateforme définit les fonctions et construit sa table ainsi :
 *
 *   #define X(id, name, arity, fn) { name, arity, fn },
 *   static const chili_native natives[] = { CHILI_NATIVES(X) };
 *   #undef X
 */
#ifndef CHILI_NATIVES_H
#define CHILI_NATIVES_H

/*         ID                 nom            arité  fonction C          */
#define CHILI_NATIVES(X)                                                  \
    X(CHILI_N_PRINT,      "print",        1, native_print)              \
    X(CHILI_N_DELAY,      "delay",        1, native_delay)              \
    X(CHILI_N_MILLIS,     "millis",       0, native_millis)             \
    X(CHILI_N_GPIO_MODE,  "gpio.mode",    2, native_gpio_mode)          \
    X(CHILI_N_GPIO_WRITE, "gpio.write",   2, native_gpio_write)         \
    X(CHILI_N_GPIO_READ,  "gpio.read",    1, native_gpio_read)          \
    X(CHILI_N_RANDOM,     "random",       2, native_random)             \
    X(CHILI_N_PRINTC,     "printc",       1, native_printc)             \
    X(CHILI_N_PRINTLN,    "println",      0, native_println)

#define CHILI_X_ENUM(id, name, arity, fn) id,
enum { CHILI_NATIVES(CHILI_X_ENUM) CHILI_NUM_NATIVES };
#undef CHILI_X_ENUM

#endif /* CHILI_NATIVES_H */

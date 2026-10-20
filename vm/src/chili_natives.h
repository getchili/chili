/*
 * chili_natives.h — table des natives, côté C.
 *
 * DOIT suivre exactement compiler/natives/natives.go (même ordre = même ID,
 * même arité, même signature). Un test Go (TestNativesTableInSync) vérifie
 * cette concordance.
 *
 * Chaque plateforme définit les fonctions et construit sa table ainsi :
 *
 *   #define X(id, name, arity, sig, fn) { name, arity, sig, fn },
 *   static const chili_native natives[] = { CHILI_NATIVES(X) };
 *   #undef X
 *
 * Les natives str.* sont fournies par la VM (chili_native_str_*, chili_vm.h).
 */
#ifndef CHILI_NATIVES_H
#define CHILI_NATIVES_H

/*         ID                 nom            arité  signature  fonction C          */
#define CHILI_NATIVES(X)                                                            \
    X(CHILI_N_PRINT,      "print",        1, "v",   native_print)                   \
    X(CHILI_N_DELAY,      "delay",        1, "i",   native_delay)                   \
    X(CHILI_N_MILLIS,     "millis",       0, "",    native_millis)                  \
    X(CHILI_N_GPIO_MODE,  "gpio.mode",    2, "ii",  native_gpio_mode)               \
    X(CHILI_N_GPIO_WRITE, "gpio.write",   2, "ii",  native_gpio_write)              \
    X(CHILI_N_GPIO_READ,  "gpio.read",    1, "i",   native_gpio_read)               \
    X(CHILI_N_RANDOM,     "random",       2, "ii",  native_random)                  \
    X(CHILI_N_PRINTC,     "printc",       1, "i",   native_printc)                  \
    X(CHILI_N_PRINTLN,    "println",      0, "",    native_println)                 \
    X(CHILI_N_STR_SUB,    "str.sub",      3, "sii", chili_native_str_sub)           \
    X(CHILI_N_STR_FIND,   "str.find",     2, "ss",  chili_native_str_find)          \
    X(CHILI_N_STR_CHAR,   "str.char",     1, "i",   chili_native_str_char)          \
    X(CHILI_N_STR_INT,    "str.int",      1, "s",   chili_native_str_int)

#define CHILI_X_ENUM(id, name, arity, sig, fn) id,
enum { CHILI_NATIVES(CHILI_X_ENUM) CHILI_NUM_NATIVES };
#undef CHILI_X_ENUM

#endif /* CHILI_NATIVES_H */

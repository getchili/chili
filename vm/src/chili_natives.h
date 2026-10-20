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
    X(CHILI_N_STR_INT,    "str.int",      1, "s",   chili_native_str_int)           \
    X(CHILI_N_PWM_WRITE,  "pwm.write",    2, "ii",  native_pwm_write)               \
    X(CHILI_N_PWM_TONE,   "pwm.tone",     2, "ii",  native_pwm_tone)                \
    X(CHILI_N_SERVO,      "servo.write",  2, "ii",  native_servo_write)             \
    X(CHILI_N_ADC_READ,   "adc.read",     1, "i",   native_adc_read)                \
    X(CHILI_N_ADC_MV,     "adc.mv",       1, "i",   native_adc_mv)                  \
    X(CHILI_N_I2C_BEGIN,  "i2c.begin",    2, "ii",  native_i2c_begin)               \
    X(CHILI_N_I2C_SCAN,   "i2c.scan",     0, "",    native_i2c_scan)                \
    X(CHILI_N_I2C_WRITE,  "i2c.write",    2, "iv",  native_i2c_write)               \
    X(CHILI_N_I2C_READ,   "i2c.read",     2, "ii",  native_i2c_read)

/* Limites des natives matériel (mêmes valeurs que compiler/natives). */
#define CHILI_PWM_MAX 255
#define CHILI_TONE_MAX 20000
#define CHILI_SERVO_MAX 180
#define CHILI_I2C_MAX_BYTES 128

/* Durée d'impulsion (µs) d'un servo pour un angle de 0 à 180°. */
#define CHILI_SERVO_PULSE(angle) (500 + (angle) * 2000 / CHILI_SERVO_MAX)

#define CHILI_X_ENUM(id, name, arity, sig, fn) id,
enum { CHILI_NATIVES(CHILI_X_ENUM) CHILI_NUM_NATIVES };
#undef CHILI_X_ENUM

#endif /* CHILI_NATIVES_H */

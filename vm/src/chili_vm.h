/*
 * chili_vm.h — machine virtuelle Chili, C99 portable.
 *
 * Exécute le bytecode décrit dans docs/bytecode.md. Comportement de référence :
 * compiler/refvm (Go). Aucune allocation dynamique : tout l'état tient dans
 * la structure chili_vm, le bytecode reste dans le tampon fourni par l'appelant.
 *
 * Utilisation :
 *
 *   chili_vm vm;
 *   if (chili_load(&vm, data, size, natives, num_natives, user) != CHILI_OK)
 *       puts(vm.error);
 *   for (;;) {
 *       chili_status st = chili_run(&vm, 1000);  // exécute au plus 1000 instructions
 *       if (st == CHILI_BUDGET) continue;        // rendre la main puis rappeler
 *       if (st == CHILI_YIELD)  ...;             // une native a demandé une pause (delay)
 *       break;                                   // CHILI_HALTED ou CHILI_ERROR
 *   }
 */
#ifndef CHILI_VM_H
#define CHILI_VM_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Version du format bytecode supportée. */
#define CHILI_BYTECODE_VERSION 3

/* Taille de la pile en valeurs : globales + locales + opérandes. */
#ifndef CHILI_STACK_SIZE
#define CHILI_STACK_SIZE 1024
#endif

/* Profondeur maximale d'appels imbriqués. */
#ifndef CHILI_MAX_FRAMES
#define CHILI_MAX_FRAMES 64
#endif

/* Nombre maximal de fonctions dans un programme. */
#ifndef CHILI_MAX_FUNCS
#define CHILI_MAX_FUNCS 256
#endif

/* Nombre maximal de classes, de noms (fonctions, classes, méthodes) et de chaînes constantes. */
#ifndef CHILI_MAX_CLASSES
#define CHILI_MAX_CLASSES 64
#endif
#ifndef CHILI_MAX_NAMES
#define CHILI_MAX_NAMES 512
#endif
#ifndef CHILI_MAX_STRINGS
#define CHILI_MAX_STRINGS 512
#endif

/* Mémoire des objets, tableaux et chaînes, en mots de 32 bits. Les blocs
 * inaccessibles sont récupérés par un ramasse-miettes quand elle est pleine. */
#ifndef CHILI_HEAP_WORDS
#define CHILI_HEAP_WORDS 8192
#endif

typedef enum {
    CHILI_OK = 0,
    CHILI_BUDGET,  /* budget d'instructions épuisé : rappeler chili_run */
    CHILI_YIELD,   /* une native demande de rendre la main (ex. delay) */
    CHILI_HALTED,  /* programme terminé normalement */
    CHILI_ERROR    /* erreur : message dans vm->error */
} chili_status;

/* Sorte d'une valeur (docs/bytecode.md §2). */
typedef enum {
    CHILI_INT = 0,
    CHILI_STR = 1,
    CHILI_ARR = 2,
    CHILI_OBJ = 3
} chili_tag;

/*
 * Valeur Chili. Pour CHILI_INT, v est l'entier. Pour une référence, v est
 * l'offset du bloc dans le tas ; pour une chaîne constante, v vaut
 * CHILI_STR_CONST | index. Les natives n'ont pas à interpréter v directement :
 * utiliser chili_str, chili_text et chili_new_str.
 */
typedef struct {
    int32_t v;
    uint8_t tag; /* chili_tag */
} chili_value;

#define CHILI_STR_CONST 0x80000000u

typedef struct chili_vm chili_vm;

/*
 * Fonction native. args contient argc valeurs, déjà vérifiées selon la
 * signature déclarée (sig). Écrire le résultat dans *ret (l'entier 0 par
 * défaut). Renvoyer :
 *   CHILI_OK    pour continuer,
 *   CHILI_YIELD pour continuer puis rendre la main à l'hôte,
 *   CHILI_ERROR après avoir appelé chili_fail().
 * Les arguments restent sur la pile de la VM pendant l'appel : une native peut
 * allouer (chili_new_str) sans risque pour eux. Une seule allocation par appel.
 */
typedef chili_status (*chili_native_fn)(chili_vm *vm, const chili_value *args, chili_value *ret);

typedef struct {
    const char *name; /* ex. "gpio.write" */
    uint8_t arity;
    const char *sig;  /* une lettre par argument : 'i' entier, 's' chaîne, 'v' toute valeur */
    chili_native_fn fn;
} chili_native;

typedef struct {
    uint32_t offset;     /* début du code de la fonction */
    uint32_t end;        /* fin (exclue) */
    uint8_t num_params;
    uint8_t num_locals;
    uint16_t max_stack;
    uint16_t name;       /* index dans la table des noms */
} chili_func;

typedef struct {
    uint16_t name;
    uint16_t parent;          /* 0xFFFF : aucune */
    uint8_t num_fields;       /* champs hérités compris */
    uint8_t abstract;
    uint16_t num_methods;
    const uint8_t *methods;   /* num_methods × (u16 nom, u16 fonction), triés par nom */
} chili_class;

typedef struct {
    uint16_t fn;
    uint32_t pc;   /* prochaine instruction (offset dans le code) */
    uint16_t base; /* index du slot 0 dans la pile */
} chili_frame;

struct chili_vm {
    /* Programme chargé. */
    const uint8_t *code;
    uint32_t code_size;
    uint16_t num_globals;
    uint16_t num_funcs;
    uint16_t entry;
    chili_func funcs[CHILI_MAX_FUNCS];
    uint16_t num_classes;
    chili_class classes[CHILI_MAX_CLASSES];
    uint16_t num_names;
    const uint8_t *names[CHILI_MAX_NAMES];     /* chaque nom : longueur (u8) puis octets */
    uint16_t num_strings;
    const uint8_t *strings[CHILI_MAX_STRINGS]; /* chaque chaîne : longueur (u16) puis octets */

    const chili_native *natives;
    uint16_t num_natives;
    void *user; /* donnée libre pour les natives */

    /* État d'exécution. Les globales occupent stack[0 .. num_globals-1]. */
    chili_value stack[CHILI_STACK_SIZE];
    uint16_t sp;
    chili_frame frames[CHILI_MAX_FRAMES];
    uint8_t num_frames;
    chili_status state;

    /* Tas : blocs [en-tête][contenu…]. En-tête = taille en mots (16 bits bas,
     * en-tête compris) | genre << 16 : 0 libre, classe + 1 pour un objet,
     * 0xFFFE chaîne, 0xFFFF tableau (docs/bytecode.md §7). */
    uint32_t heap[CHILI_HEAP_WORDS];
    uint8_t heap_tags[(CHILI_HEAP_WORDS + 3) / 4];      /* sorte de chaque mot, 2 bits */
    uint32_t heap_starts[(CHILI_HEAP_WORDS + 31) / 32]; /* début de bloc */
    uint32_t heap_marks[(CHILI_HEAP_WORDS + 31) / 32];  /* ramasse-miettes */
    uint32_t gc_runs;

    char error[160];
};

/*
 * Vérifie et charge un fichier .chlb (en-tête, CRC, contrôles du §8 de
 * docs/bytecode.md, natives connues avec la bonne arité), puis prépare
 * l'exécution depuis la fonction d'entrée. data doit rester valide tant que
 * la VM l'utilise. Renvoie CHILI_OK ou CHILI_ERROR (message dans vm->error).
 */
chili_status chili_load(chili_vm *vm, const uint8_t *data, size_t size,
                        const chili_native *natives, uint16_t num_natives, void *user);

/* Remet le programme chargé à son état initial (globales à 0, entrée). */
chili_status chili_reset(chili_vm *vm);

/* Exécute au plus max_steps instructions (0 = sans limite). */
chili_status chili_run(chili_vm *vm, uint32_t max_steps);

/* Pour les natives : signale une erreur d'exécution. Renvoie CHILI_ERROR. */
chili_status chili_fail(chili_vm *vm, const char *fmt, ...);

/* CRC-32 IEEE (même algorithme que Go hash/crc32.ChecksumIEEE). */
uint32_t chili_crc32(const uint8_t *data, size_t size);

/* ------------------------------------------------------------------------ */
/* Valeurs, pour les natives                                                */

/* Construit un entier. */
static inline chili_value chili_int(int32_t v)
{
    chili_value r;
    r.v = v;
    r.tag = CHILI_INT;
    return r;
}

/* Octets d'une chaîne (non terminée par 0) et longueur dans *len, ou NULL si v n'est pas une chaîne. */
const uint8_t *chili_str(const chili_vm *vm, chili_value v, uint32_t *len);

/*
 * Crée une chaîne de len octets (copiés depuis bytes, qui peut être NULL pour
 * des zéros) et la place dans *out. Peut lancer le ramasse-miettes.
 * Renvoie CHILI_ERROR (via chili_fail) si la mémoire est pleine.
 */
chili_status chili_new_str(chili_vm *vm, const uint8_t *bytes, uint32_t len, chili_value *out);

/*
 * Texte d'une valeur, tel que l'affiche print : renvoie un pointeur vers les
 * octets (dans buf ou dans la chaîne elle-même) et leur nombre dans *len.
 * buf doit faire au moins 64 octets.
 */
const char *chili_text(const chili_vm *vm, chili_value v, char *buf, size_t size, uint32_t *len);

/*
 * Pour la native random(min, max) : ramène une valeur aléatoire 32 bits r
 * dans [min, max], bornes incluses (même formule que natives.RandomRange en Go).
 * Renvoie CHILI_ERROR (via chili_fail) si max < min.
 */
chili_status chili_random_range(chili_vm *vm, int32_t min, int32_t max, uint32_t r, int32_t *out);

/*
 * Pour la native printc(code) : encode le caractère Unicode code en UTF-8
 * dans out (4 octets max) et renvoie le nombre d'octets. Renvoie 0 après
 * chili_fail si le code n'est pas un caractère valide.
 */
int chili_utf8(chili_vm *vm, int32_t code, char out[4]);

/* Natives str.* fournies par la VM : à placer telles quelles dans la table de la plateforme. */
chili_status chili_native_str_sub(chili_vm *vm, const chili_value *args, chili_value *ret);
chili_status chili_native_str_find(chili_vm *vm, const chili_value *args, chili_value *ret);
chili_status chili_native_str_char(chili_vm *vm, const chili_value *args, chili_value *ret);
chili_status chili_native_str_int(chili_vm *vm, const chili_value *args, chili_value *ret);

#ifdef __cplusplus
}
#endif

#endif /* CHILI_VM_H */

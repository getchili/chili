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
#define CHILI_BYTECODE_VERSION 2

/* Taille de la pile en valeurs 32 bits : globales + locales + opérandes. */
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

/* Nombre maximal de classes et de noms (fonctions, classes, méthodes). */
#ifndef CHILI_MAX_CLASSES
#define CHILI_MAX_CLASSES 64
#endif
#ifndef CHILI_MAX_NAMES
#define CHILI_MAX_NAMES 512
#endif

/* Mémoire des objets, en mots de 32 bits (en-tête + champs de chaque objet).
 * Les objets inaccessibles sont récupérés par un ramasse-miettes quand elle est pleine. */
#ifndef CHILI_HEAP_WORDS
#define CHILI_HEAP_WORDS 4096
#endif

typedef enum {
    CHILI_OK = 0,
    CHILI_BUDGET,  /* budget d'instructions épuisé : rappeler chili_run */
    CHILI_YIELD,   /* une native demande de rendre la main (ex. delay) */
    CHILI_HALTED,  /* programme terminé normalement */
    CHILI_ERROR    /* erreur : message dans vm->error */
} chili_status;

typedef struct chili_vm chili_vm;

/*
 * Fonction native. args contient argc valeurs (argc = arité déclarée).
 * Écrire le résultat dans *ret (0 par défaut). Renvoyer :
 *   CHILI_OK    pour continuer,
 *   CHILI_YIELD pour continuer puis rendre la main à l'hôte,
 *   CHILI_ERROR après avoir appelé chili_fail().
 */
typedef chili_status (*chili_native_fn)(chili_vm *vm, const int32_t *args, int32_t *ret);

typedef struct {
    const char *name; /* ex. "gpio.write" */
    uint8_t arity;
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
    const uint8_t *names[CHILI_MAX_NAMES]; /* chaque nom : longueur (u8) puis octets */

    const chili_native *natives;
    uint16_t num_natives;
    void *user; /* donnée libre pour les natives */

    /* État d'exécution. Les globales occupent stack[0 .. num_globals-1]. */
    int32_t stack[CHILI_STACK_SIZE];
    uint16_t sp;
    chili_frame frames[CHILI_MAX_FRAMES];
    uint8_t num_frames;
    chili_status state;

    /* Objets : blocs [en-tête][champs…]. En-tête = taille (16 bits bas,
     * en-tête compris) | (classe + 1) << 16, classe + 1 = 0 pour un bloc libre. */
    uint32_t heap[CHILI_HEAP_WORDS];
    uint32_t heap_starts[(CHILI_HEAP_WORDS + 31) / 32]; /* début de bloc */
    uint32_t heap_marks[(CHILI_HEAP_WORDS + 31) / 32];  /* ramasse-miettes */
    uint32_t gc_runs;

    char error[128];
};

/*
 * Vérifie et charge un fichier .chlb (en-tête, CRC, contrôles du §4 de
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

/*
 * Pour la native random(min, max) : ramène une valeur aléatoire 32 bits r
 * dans [min, max], bornes incluses (même formule que natives.RandomRange en Go).
 * Renvoie CHILI_ERROR (via chili_fail) si max < min.
 */
chili_status chili_random_range(chili_vm *vm, int32_t min, int32_t max, uint32_t r, int32_t *out);

#ifdef __cplusplus
}
#endif

#endif /* CHILI_VM_H */

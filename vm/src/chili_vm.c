/*
 * chili_vm.c — implémentation de la VM Chili (cf. chili_vm.h).
 */
#include "chili_vm.h"

#include <stdarg.h>
#include <stdio.h>
#include <string.h>

/* Opcodes (docs/bytecode.md §3). */
enum {
    OP_HALT = 0x00, OP_PUSH = 0x01, OP_PUSH8 = 0x02, OP_POP = 0x03,
    OP_LOAD = 0x10, OP_STORE = 0x11, OP_GLOAD = 0x12, OP_GSTORE = 0x13,
    OP_ADD = 0x20, OP_SUB = 0x21, OP_MUL = 0x22, OP_DIV = 0x23, OP_MOD = 0x24,
    OP_AND = 0x25, OP_OR = 0x26, OP_XOR = 0x27, OP_SHL = 0x28, OP_SHR = 0x29,
    OP_NEG = 0x2A, OP_NOT = 0x2B, OP_BNOT = 0x2C,
    OP_EQ = 0x30, OP_NE = 0x31, OP_LT = 0x32, OP_LE = 0x33, OP_GT = 0x34, OP_GE = 0x35,
    OP_JMP = 0x40, OP_JZ = 0x41, OP_JNZ = 0x42,
    OP_CALL = 0x50, OP_CALL_NATIVE = 0x51, OP_RET = 0x52,
    OP_ALLOC = 0x60, OP_GETF = 0x61, OP_SETF = 0x62, OP_CALLM = 0x63
};

#define HEADER_SIZE 20
#define FUNC_SIZE 10
#define CLASS_SIZE 8
#define NO_PARENT 0xFFFF

#define HANDLE_TAG 0x4F000000u

/* Taille totale de chaque instruction (opcode + opérandes), 0 = opcode inconnu. */
static uint8_t op_size(uint8_t op)
{
    switch (op) {
    case OP_HALT: case OP_POP:
    case OP_ADD: case OP_SUB: case OP_MUL: case OP_DIV: case OP_MOD:
    case OP_AND: case OP_OR: case OP_XOR: case OP_SHL: case OP_SHR:
    case OP_NEG: case OP_NOT: case OP_BNOT:
    case OP_EQ: case OP_NE: case OP_LT: case OP_LE: case OP_GT: case OP_GE:
    case OP_RET:
        return 1;
    case OP_PUSH8: case OP_LOAD: case OP_STORE: case OP_GETF: case OP_SETF:
        return 2;
    case OP_GLOAD: case OP_GSTORE: case OP_JMP: case OP_JZ: case OP_JNZ: case OP_CALL:
    case OP_ALLOC:
        return 3;
    case OP_CALL_NATIVE: case OP_CALLM:
        return 4;
    case OP_PUSH:
        return 5;
    default:
        return 0;
    }
}

static uint16_t rd16(const uint8_t *p) { return (uint16_t)(p[0] | (p[1] << 8)); }
static uint32_t rd32(const uint8_t *p)
{
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

uint32_t chili_crc32(const uint8_t *data, size_t size)
{
    uint32_t crc = 0xFFFFFFFFu;
    for (size_t i = 0; i < size; i++) {
        crc ^= data[i];
        for (int k = 0; k < 8; k++)
            crc = (crc >> 1) ^ (0xEDB88320u & (0u - (crc & 1u)));
    }
    return ~crc;
}

chili_status chili_random_range(chili_vm *vm, int32_t min, int32_t max, uint32_t r, int32_t *out)
{
    if (max < min)
        return chili_fail(vm, "random : le maximum (%ld) est plus petit que le minimum (%ld)",
                          (long)max, (long)min);
    uint64_t span = (uint64_t)((int64_t)max - (int64_t)min) + 1u; /* de 1 à 2^32 */
    *out = (int32_t)((int64_t)min + (int64_t)(((uint64_t)r * span) >> 32));
    return CHILI_OK;
}

chili_status chili_fail(chili_vm *vm, const char *fmt, ...)
{
    va_list ap;
    va_start(ap, fmt);
    vsnprintf(vm->error, sizeof vm->error, fmt, ap);
    va_end(ap);
    vm->state = CHILI_ERROR;
    return CHILI_ERROR;
}

/* Nom d'index i : longueur dans *len, pointeur renvoyé ("?" si invalide). */
static const char *name_of(const chili_vm *vm, uint16_t i, int *len)
{
    if (i >= vm->num_names) {
        *len = 1;
        return "?";
    }
    *len = vm->names[i][0];
    return (const char *)vm->names[i] + 1;
}

/* ------------------------------------------------------------------------ */
/* Objets et ramasse-miettes                                                */

#define BIT_GET(set, i) (((set)[(i) >> 5] >> ((i) & 31)) & 1u)
#define BIT_SET(set, i) ((set)[(i) >> 5] |= 1u << ((i) & 31))
#define BIT_CLR(set, i) ((set)[(i) >> 5] &= ~(1u << ((i) & 31)))

#define BLK_SIZE(h) ((h) & 0xFFFFu)
#define BLK_CLASS1(h) ((h) >> 16) /* classe + 1, 0 = libre */

static void heap_init(chili_vm *vm)
{
    memset(vm->heap_starts, 0, sizeof vm->heap_starts);
    memset(vm->heap_marks, 0, sizeof vm->heap_marks);
    vm->heap[0] = CHILI_HEAP_WORDS; /* un seul bloc libre */
    BIT_SET(vm->heap_starts, 0);
    vm->gc_runs = 0;
}

/* Si v désigne un objet vivant, renvoie 1 et son offset dans *off. Une valeur
 * entière qui ressemble par hasard à un objet est traitée comme telle
 * (ramasse-miettes conservatif) : au pire, l'objet est gardé plus longtemps. */
static int as_object(const chili_vm *vm, int32_t v, uint32_t *off)
{
    uint32_t u = (uint32_t)v;
    if ((u & 0xFF000000u) != HANDLE_TAG)
        return 0;
    uint32_t o = u & 0xFFFFFFu;
    if (o >= CHILI_HEAP_WORDS || !BIT_GET(vm->heap_starts, o) || BLK_CLASS1(vm->heap[o]) == 0)
        return 0;
    *off = o;
    return 1;
}

static void mark_value(chili_vm *vm, int32_t v, int *changed)
{
    uint32_t o;
    if (as_object(vm, v, &o) && !BIT_GET(vm->heap_marks, o)) {
        BIT_SET(vm->heap_marks, o);
        *changed = 1;
    }
}

static void gc(chili_vm *vm)
{
    int changed = 0;
    memset(vm->heap_marks, 0, sizeof vm->heap_marks);

    /* Racines : globales, locales et valeurs temporaires de tous les cadres. */
    for (uint32_t i = 0; i < vm->sp; i++)
        mark_value(vm, vm->stack[i], &changed);

    /* Propagation : les champs des objets marqués, jusqu'à stabilité. */
    while (changed) {
        changed = 0;
        for (uint32_t o = 0; o < CHILI_HEAP_WORDS; o += BLK_SIZE(vm->heap[o])) {
            uint32_t h = vm->heap[o];
            if (BLK_CLASS1(h) && BIT_GET(vm->heap_marks, o))
                for (uint32_t f = 1; f < BLK_SIZE(h); f++)
                    mark_value(vm, (int32_t)vm->heap[o + f], &changed);
        }
    }

    /* Balayage : libère les objets non marqués et fusionne les blocs libres voisins. */
    uint32_t prev_free = CHILI_HEAP_WORDS; /* aucun */
    for (uint32_t o = 0; o < CHILI_HEAP_WORDS;) {
        uint32_t size = BLK_SIZE(vm->heap[o]);
        if (BLK_CLASS1(vm->heap[o]) && !BIT_GET(vm->heap_marks, o))
            vm->heap[o] = size; /* libre */
        if (BLK_CLASS1(vm->heap[o]) == 0) {
            if (prev_free != CHILI_HEAP_WORDS) {
                vm->heap[prev_free] = BLK_SIZE(vm->heap[prev_free]) + size;
                BIT_CLR(vm->heap_starts, o);
            } else {
                prev_free = o;
            }
        } else {
            prev_free = CHILI_HEAP_WORDS;
        }
        o += size;
    }
    memset(vm->heap_marks, 0, sizeof vm->heap_marks);
    vm->gc_runs++;
}

/* Premier bloc libre assez grand : renvoie son offset, ou CHILI_HEAP_WORDS. */
static uint32_t find_free(chili_vm *vm, uint32_t need)
{
    for (uint32_t o = 0; o < CHILI_HEAP_WORDS; o += BLK_SIZE(vm->heap[o]))
        if (BLK_CLASS1(vm->heap[o]) == 0 && BLK_SIZE(vm->heap[o]) >= need)
            return o;
    return CHILI_HEAP_WORDS;
}

/* Crée un objet de la classe c (champs à 0). Renvoie 0 si la mémoire est pleine. */
static int32_t alloc_object(chili_vm *vm, uint16_t c)
{
    uint32_t need = (uint32_t)vm->classes[c].num_fields + 1;
    uint32_t o = find_free(vm, need);
    if (o == CHILI_HEAP_WORDS) {
        gc(vm);
        o = find_free(vm, need);
        if (o == CHILI_HEAP_WORDS)
            return 0;
    }
    uint32_t size = BLK_SIZE(vm->heap[o]);
    if (size > need) { /* découpe : le reste forme un bloc libre */
        vm->heap[o + need] = size - need;
        BIT_SET(vm->heap_starts, o + need);
        size = need;
    }
    vm->heap[o] = size | ((uint32_t)(c + 1) << 16);
    memset(&vm->heap[o + 1], 0, (size - 1) * sizeof vm->heap[0]);
    return (int32_t)(HANDLE_TAG | o);
}

/* ------------------------------------------------------------------------ */
/* Chargement et vérification                                               */

static chili_status verify_func(chili_vm *vm, uint16_t fi)
{
    const chili_func *f = &vm->funcs[fi];
    const uint8_t *code = vm->code;
    uint32_t pc = f->offset;
    uint8_t last = 0xFF;

    /* 1re passe : opcodes et opérandes. */
    while (pc < f->end) {
        uint8_t op = code[pc];
        uint8_t n = op_size(op);
        if (n == 0)
            return chili_fail(vm, "f%u : opcode inconnu 0x%02X a %lu", fi, op, (unsigned long)pc);
        if (pc + n > f->end)
            return chili_fail(vm, "f%u : instruction tronquee a %lu", fi, (unsigned long)pc);
        const uint8_t *a = code + pc + 1;
        switch (op) {
        case OP_LOAD: case OP_STORE:
            if (a[0] >= f->num_locals)
                return chili_fail(vm, "f%u : slot %u hors limites", fi, a[0]);
            break;
        case OP_GLOAD: case OP_GSTORE:
            if (rd16(a) >= vm->num_globals)
                return chili_fail(vm, "f%u : globale %u hors limites", fi, rd16(a));
            break;
        case OP_CALL:
            if (rd16(a) >= vm->num_funcs)
                return chili_fail(vm, "f%u : fonction %u inexistante", fi, rd16(a));
            break;
        case OP_CALL_NATIVE: {
            uint16_t id = rd16(a);
            if (id >= vm->num_natives || vm->natives[id].fn == NULL)
                return chili_fail(vm, "native %u non disponible sur cette carte", id);
            if (vm->natives[id].arity != a[2])
                return chili_fail(vm, "native %s : %u argument(s) attendu(s), %u fourni(s)",
                                  vm->natives[id].name, vm->natives[id].arity, a[2]);
            break;
        }
        case OP_ALLOC:
            if (rd16(a) >= vm->num_classes || vm->classes[rd16(a)].abstract)
                return chili_fail(vm, "f%u : classe %u invalide", fi, rd16(a));
            break;
        case OP_CALLM:
            if (rd16(a) >= vm->num_names)
                return chili_fail(vm, "f%u : nom %u invalide", fi, rd16(a));
            break;
        default:
            break;
        }
        last = op;
        pc += n;
    }
    if (last != OP_RET && last != OP_JMP && last != OP_HALT)
        return chili_fail(vm, "f%u : doit se terminer par RET, JMP ou HALT", fi);

    /* 2e passe : chaque cible de saut doit être un début d'instruction. */
    for (pc = f->offset; pc < f->end; pc += op_size(code[pc])) {
        uint8_t op = code[pc];
        if (op != OP_JMP && op != OP_JZ && op != OP_JNZ)
            continue;
        int32_t target = (int32_t)(pc + 3) + (int16_t)rd16(code + pc + 1);
        uint32_t q = f->offset;
        while ((int32_t)q < target && q < f->end)
            q += op_size(code[q]);
        if (target < (int32_t)f->offset || (int32_t)q != target || q >= f->end)
            return chili_fail(vm, "f%u : saut invalide a %lu", fi, (unsigned long)pc);
    }
    return CHILI_OK;
}

chili_status chili_load(chili_vm *vm, const uint8_t *data, size_t size,
                        const chili_native *natives, uint16_t num_natives, void *user)
{
    memset(vm, 0, sizeof *vm);
    vm->natives = natives;
    vm->num_natives = num_natives;
    vm->user = user;
    vm->state = CHILI_ERROR;

    if (size < HEADER_SIZE + 4)
        return chili_fail(vm, "fichier trop court");
    if (memcmp(data, "CHLB", 4) != 0)
        return chili_fail(vm, "ce n'est pas un fichier .chlb");
    if (data[4] != CHILI_BYTECODE_VERSION)
        return chili_fail(vm, "version de bytecode %u non supportee (attendu %u) : recompile le programme",
                          data[4], CHILI_BYTECODE_VERSION);
    if (chili_crc32(data, size - 4) != rd32(data + size - 4))
        return chili_fail(vm, "CRC invalide : fichier corrompu");

    size_t body = size - 4;
    uint16_t num_funcs = rd16(data + 8);
    uint32_t code_size = rd32(data + 12);
    uint16_t num_classes = rd16(data + 16);
    uint16_t num_names = rd16(data + 18);
    if (num_funcs == 0 || num_funcs > CHILI_MAX_FUNCS)
        return chili_fail(vm, "nombre de fonctions invalide (%u, max %u)", num_funcs, CHILI_MAX_FUNCS);
    if (num_classes > CHILI_MAX_CLASSES)
        return chili_fail(vm, "trop de classes (%u, max %u)", num_classes, CHILI_MAX_CLASSES);
    if (num_names > CHILI_MAX_NAMES)
        return chili_fail(vm, "trop de noms (%u, max %u)", num_names, CHILI_MAX_NAMES);

    vm->num_globals = rd16(data + 6);
    vm->num_funcs = num_funcs;
    vm->entry = rd16(data + 10);
    vm->num_classes = num_classes;
    vm->num_names = num_names;

    size_t off = HEADER_SIZE;
#define NEED(n) do { if (off + (n) > body) return chili_fail(vm, "fichier tronque"); } while (0)
    NEED((size_t)num_funcs * FUNC_SIZE);
    for (uint16_t i = 0; i < num_funcs; i++, off += FUNC_SIZE) {
        const uint8_t *e = data + off;
        chili_func *f = &vm->funcs[i];
        f->offset = rd32(e);
        f->num_params = e[4];
        f->num_locals = e[5];
        f->max_stack = rd16(e + 6);
        f->name = rd16(e + 8);
    }
    for (uint16_t i = 0; i < num_classes; i++) {
        NEED(CLASS_SIZE);
        const uint8_t *e = data + off;
        chili_class *c = &vm->classes[i];
        c->name = rd16(e);
        c->parent = rd16(e + 2);
        c->num_fields = e[4];
        c->abstract = e[5] & 1;
        c->num_methods = rd16(e + 6);
        off += CLASS_SIZE;
        NEED((size_t)c->num_methods * 4);
        c->methods = data + off;
        off += (size_t)c->num_methods * 4;
    }
    for (uint16_t i = 0; i < num_names; i++) {
        NEED(1);
        vm->names[i] = data + off;
        NEED(1 + (size_t)data[off]);
        off += 1 + data[off];
    }
#undef NEED
    if (off + code_size != body)
        return chili_fail(vm, "taille incoherente (%lu octets)", (unsigned long)size);
    vm->code = data + off;
    vm->code_size = code_size;

    if (vm->entry >= num_funcs)
        return chili_fail(vm, "fonction d'entree inexistante");
    if (vm->num_globals >= CHILI_STACK_SIZE)
        return chili_fail(vm, "trop de globales pour la pile (%u)", vm->num_globals);

    for (uint16_t i = 0; i < num_classes; i++) {
        const chili_class *c = &vm->classes[i];
        if (c->name >= num_names || (c->parent != NO_PARENT && c->parent >= num_classes))
            return chili_fail(vm, "classe %u invalide", i);
        for (uint16_t m = 0; m < c->num_methods; m++) {
            uint16_t sel = rd16(c->methods + 4 * m), fn = rd16(c->methods + 4 * m + 2);
            if (sel >= num_names || fn >= num_funcs || vm->funcs[fn].num_params == 0 ||
                (m > 0 && rd16(c->methods + 4 * (m - 1)) >= sel))
                return chili_fail(vm, "classe %u : table de methodes invalide", i);
        }
    }
    for (uint16_t i = 0; i < num_funcs; i++) {
        chili_func *f = &vm->funcs[i];
        f->end = (i + 1 < num_funcs) ? vm->funcs[i + 1].offset : code_size;
        if ((i == 0 && f->offset != 0) || f->offset >= f->end || f->end > code_size)
            return chili_fail(vm, "f%u : bornes de code invalides", i);
        if (f->num_params > f->num_locals)
            return chili_fail(vm, "f%u : plus de parametres que de locales", i);
        if (num_names > 0 && f->name >= num_names)
            return chili_fail(vm, "f%u : nom invalide", i);
    }
    for (uint16_t i = 0; i < num_funcs; i++)
        if (verify_func(vm, i) != CHILI_OK)
            return CHILI_ERROR;

    return chili_reset(vm);
}

/* ------------------------------------------------------------------------ */
/* Exécution                                                                */

/* Crée le cadre de fn ; ses paramètres sont déjà au sommet de la pile. */
static chili_status enter(chili_vm *vm, uint16_t fn)
{
    const chili_func *f = &vm->funcs[fn];
    if (vm->num_frames >= CHILI_MAX_FRAMES)
        return chili_fail(vm, "trop d'appels imbriques (recursion trop profonde ?)");
    uint32_t base = (uint32_t)vm->sp - f->num_params;
    if (base + f->num_locals + f->max_stack > CHILI_STACK_SIZE)
        return chili_fail(vm, "depassement de pile");
    for (unsigned i = f->num_params; i < f->num_locals; i++)
        vm->stack[vm->sp++] = 0;
    chili_frame *fr = &vm->frames[vm->num_frames++];
    fr->fn = fn;
    fr->pc = f->offset;
    fr->base = (uint16_t)base;
    return CHILI_OK;
}

chili_status chili_reset(chili_vm *vm)
{
    memset(vm->stack, 0, sizeof vm->stack);
    heap_init(vm);
    vm->sp = vm->num_globals;
    vm->num_frames = 0;
    vm->error[0] = '\0';
    vm->state = CHILI_OK;
    if (enter(vm, vm->entry) != CHILI_OK)
        return CHILI_ERROR;
    return CHILI_OK;
}

/* Cherche la méthode sel dans la classe c (recherche dichotomique). */
static int find_method(const chili_vm *vm, uint16_t c, uint16_t sel, uint16_t *fn)
{
    const chili_class *k = &vm->classes[c];
    int lo = 0, hi = (int)k->num_methods - 1;
    while (lo <= hi) {
        int mid = (lo + hi) / 2;
        uint16_t s = rd16(k->methods + 4 * mid);
        if (s == sel) {
            *fn = rd16(k->methods + 4 * mid + 2);
            return 1;
        }
        if (s < sel)
            lo = mid + 1;
        else
            hi = mid - 1;
    }
    return 0;
}

#define PUSH(v) (vm->stack[vm->sp++] = (v))
#define POP() (vm->stack[--vm->sp])
#define TOP (vm->stack[vm->sp - 1])

chili_status chili_run(chili_vm *vm, uint32_t max_steps)
{
    if (vm->state == CHILI_HALTED || vm->state == CHILI_ERROR)
        return vm->state;
    vm->state = CHILI_OK;

    const uint8_t *code = vm->code;
    uint32_t steps = 0;

    for (;;) {
        if (max_steps && steps++ >= max_steps)
            return CHILI_BUDGET;

        chili_frame *fr = &vm->frames[vm->num_frames - 1];
        uint32_t pc = fr->pc;
        uint8_t op = code[pc];
        const uint8_t *a = code + pc + 1;
        fr->pc = pc + op_size(op);

#define FAIL(...) do { \
        char msg_[96]; \
        int fn_len_; \
        const char *fn_name_ = name_of(vm, vm->funcs[fr->fn].name, &fn_len_); \
        snprintf(msg_, sizeof msg_, __VA_ARGS__); \
        return chili_fail(vm, "%s (dans %.*s, pc %04lx)", msg_, fn_len_, fn_name_, (unsigned long)pc); \
    } while (0)

        switch (op) {
        case OP_HALT:
            vm->state = CHILI_HALTED;
            return CHILI_HALTED;

        case OP_PUSH:  PUSH((int32_t)rd32(a)); break;
        case OP_PUSH8: PUSH((int8_t)a[0]); break;
        case OP_POP:   vm->sp--; break;

        case OP_LOAD:   PUSH(vm->stack[fr->base + a[0]]); break;
        case OP_STORE:  vm->stack[fr->base + a[0]] = POP(); break;
        case OP_GLOAD:  PUSH(vm->stack[rd16(a)]); break;
        case OP_GSTORE: vm->stack[rd16(a)] = POP(); break;

        /* Arithmétique modulo 2^32 : on calcule en non signé pour éviter
         * tout comportement indéfini du C sur les débordements. */
        case OP_ADD: { uint32_t b = (uint32_t)POP(); TOP = (int32_t)((uint32_t)TOP + b); break; }
        case OP_SUB: { uint32_t b = (uint32_t)POP(); TOP = (int32_t)((uint32_t)TOP - b); break; }
        case OP_MUL: { uint32_t b = (uint32_t)POP(); TOP = (int32_t)((uint32_t)TOP * b); break; }
        case OP_DIV: case OP_MOD: {
            int32_t b = POP();
            int32_t x = TOP;
            if (b == 0)
                FAIL("%s", op == OP_DIV ? "division par zero" : "modulo par zero");
            if (x == INT32_MIN && b == -1)
                TOP = (op == OP_DIV) ? INT32_MIN : 0;
            else
                TOP = (op == OP_DIV) ? x / b : x % b;
            break;
        }
        case OP_AND: { int32_t b = POP(); TOP &= b; break; }
        case OP_OR:  { int32_t b = POP(); TOP |= b; break; }
        case OP_XOR: { int32_t b = POP(); TOP ^= b; break; }
        case OP_SHL: { uint32_t b = (uint32_t)POP() & 31u; TOP = (int32_t)((uint32_t)TOP << b); break; }
        case OP_SHR: {
            /* Décalage arithmétique, sans dépendre du comportement du compilateur. */
            uint32_t b = (uint32_t)POP() & 31u;
            int32_t x = TOP;
            TOP = (x >= 0) ? (int32_t)((uint32_t)x >> b) : (int32_t)~(~(uint32_t)x >> b);
            break;
        }
        case OP_NEG:  TOP = (int32_t)(0u - (uint32_t)TOP); break;
        case OP_NOT:  TOP = (TOP == 0); break;
        case OP_BNOT: TOP = ~TOP; break;

        case OP_EQ: { int32_t b = POP(); TOP = (TOP == b); break; }
        case OP_NE: { int32_t b = POP(); TOP = (TOP != b); break; }
        case OP_LT: { int32_t b = POP(); TOP = (TOP < b); break; }
        case OP_LE: { int32_t b = POP(); TOP = (TOP <= b); break; }
        case OP_GT: { int32_t b = POP(); TOP = (TOP > b); break; }
        case OP_GE: { int32_t b = POP(); TOP = (TOP >= b); break; }

        case OP_JMP: fr->pc += (int16_t)rd16(a); break;
        case OP_JZ:  if (POP() == 0) fr->pc += (int16_t)rd16(a); break;
        case OP_JNZ: if (POP() != 0) fr->pc += (int16_t)rd16(a); break;

        case OP_CALL:
            if (enter(vm, rd16(a)) != CHILI_OK) {
                char msg[sizeof vm->error];
                memcpy(msg, vm->error, sizeof msg);
                FAIL("%.80s", msg);
            }
            break;

        case OP_CALL_NATIVE: {
            const chili_native *nat = &vm->natives[rd16(a)];
            uint8_t argc = a[2];
            int32_t ret = 0;
            vm->sp -= argc;
            chili_status st = nat->fn(vm, &vm->stack[vm->sp], &ret);
            PUSH(ret);
            if (st == CHILI_ERROR)
                return CHILI_ERROR;
            if (st == CHILI_YIELD)
                return CHILI_YIELD;
            break;
        }

        case OP_RET: {
            int32_t r = POP();
            vm->sp = fr->base;
            if (--vm->num_frames == 0) {
                vm->state = CHILI_HALTED;
                return CHILI_HALTED;
            }
            PUSH(r);
            break;
        }

        case OP_ALLOC: {
            int32_t h = alloc_object(vm, rd16(a));
            if (h == 0)
                FAIL("memoire des objets pleine (%u mots)", (unsigned)CHILI_HEAP_WORDS);
            PUSH(h);
            break;
        }

        case OP_GETF: case OP_SETF: {
            int32_t val = 0;
            uint32_t o;
            if (op == OP_SETF)
                val = POP();
            if (!as_object(vm, POP(), &o) || a[0] + 1u >= BLK_SIZE(vm->heap[o]))
                FAIL("acces a un champ sur une valeur qui n'est pas un objet");
            if (op == OP_SETF)
                vm->heap[o + 1 + a[0]] = (uint32_t)val;
            else
                PUSH((int32_t)vm->heap[o + 1 + a[0]]);
            break;
        }

        case OP_CALLM: {
            uint16_t sel = rd16(a), fn;
            uint8_t argc = a[2];
            uint32_t o;
            int sel_len, cls_len;
            const char *sel_name = name_of(vm, sel, &sel_len);
            if (!as_object(vm, vm->stack[vm->sp - argc - 1], &o))
                FAIL("appel de %.*s() sur une valeur qui n'est pas un objet", sel_len, sel_name);
            uint16_t c = (uint16_t)(BLK_CLASS1(vm->heap[o]) - 1);
            const char *cls_name = name_of(vm, vm->classes[c].name, &cls_len);
            if (!find_method(vm, c, sel, &fn))
                FAIL("la classe %.*s n'a pas de methode %.*s", cls_len, cls_name, sel_len, sel_name);
            if (vm->funcs[fn].num_params != argc + 1u)
                FAIL("%.*s.%.*s attend %u argument(s), %u fourni(s)", cls_len, cls_name, sel_len, sel_name,
                     (unsigned)(vm->funcs[fn].num_params - 1), (unsigned)argc);
            if (enter(vm, fn) != CHILI_OK) {
                char msg[sizeof vm->error];
                memcpy(msg, vm->error, sizeof msg);
                FAIL("%.80s", msg);
            }
            break;
        }

        default:
            FAIL("opcode inconnu");
        }
#undef FAIL
    }
}

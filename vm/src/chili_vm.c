/*
 * chili_vm.c — implémentation de la VM Chili (cf. chili_vm.h).
 */
#include "chili_vm.h"

#include <stdarg.h>
#include <stdio.h>
#include <string.h>

/* Opcodes (docs/bytecode.md §4). */
enum {
    OP_HALT = 0x00, OP_PUSH = 0x01, OP_PUSH8 = 0x02, OP_POP = 0x03, OP_PUSHS = 0x04, OP_DUP2 = 0x05,
    OP_LOAD = 0x10, OP_STORE = 0x11, OP_GLOAD = 0x12, OP_GSTORE = 0x13,
    OP_ADD = 0x20, OP_SUB = 0x21, OP_MUL = 0x22, OP_DIV = 0x23, OP_MOD = 0x24,
    OP_AND = 0x25, OP_OR = 0x26, OP_XOR = 0x27, OP_SHL = 0x28, OP_SHR = 0x29,
    OP_NEG = 0x2A, OP_NOT = 0x2B, OP_BNOT = 0x2C,
    OP_EQ = 0x30, OP_NE = 0x31, OP_LT = 0x32, OP_LE = 0x33, OP_GT = 0x34, OP_GE = 0x35,
    OP_JMP = 0x40, OP_JZ = 0x41, OP_JNZ = 0x42,
    OP_CALL = 0x50, OP_CALL_NATIVE = 0x51, OP_RET = 0x52,
    OP_ALLOC = 0x60, OP_GETF = 0x61, OP_SETF = 0x62, OP_CALLM = 0x63,
    OP_NEWARR = 0x64, OP_MKARR = 0x65, OP_GETI = 0x66, OP_SETI = 0x67, OP_LEN = 0x68
};

#define HEADER_SIZE 22
#define FUNC_SIZE 10
#define CLASS_SIZE 8
#define NO_PARENT 0xFFFF

/* Genre d'un bloc du tas (16 bits de poids fort de l'en-tête). */
#define KIND_FREE 0u
#define KIND_STR 0xFFFEu
#define KIND_ARR 0xFFFFu

/* Taille totale de chaque instruction (opcode + opérandes), 0 = opcode inconnu. */
static uint8_t op_size(uint8_t op)
{
    switch (op) {
    case OP_HALT: case OP_POP: case OP_DUP2:
    case OP_ADD: case OP_SUB: case OP_MUL: case OP_DIV: case OP_MOD:
    case OP_AND: case OP_OR: case OP_XOR: case OP_SHL: case OP_SHR:
    case OP_NEG: case OP_NOT: case OP_BNOT:
    case OP_EQ: case OP_NE: case OP_LT: case OP_LE: case OP_GT: case OP_GE:
    case OP_RET: case OP_NEWARR: case OP_GETI: case OP_SETI: case OP_LEN:
        return 1;
    case OP_PUSH8: case OP_LOAD: case OP_STORE: case OP_GETF: case OP_SETF:
        return 2;
    case OP_GLOAD: case OP_GSTORE: case OP_JMP: case OP_JZ: case OP_JNZ: case OP_CALL:
    case OP_ALLOC: case OP_PUSHS: case OP_MKARR:
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

static int utf8_encode(chili_vm *vm, const char *who, int32_t code, char out[4])
{
    if (code < 1 || code > 0x10FFFF || (code >= 0xD800 && code <= 0xDFFF)) {
        chili_fail(vm, "%s : %ld n'est pas un code de caractere valide", who, (long)code);
        return 0;
    }
    uint32_t c = (uint32_t)code;
    if (c < 0x80) {
        out[0] = (char)c;
        return 1;
    }
    if (c < 0x800) {
        out[0] = (char)(0xC0 | (c >> 6));
        out[1] = (char)(0x80 | (c & 0x3F));
        return 2;
    }
    if (c < 0x10000) {
        out[0] = (char)(0xE0 | (c >> 12));
        out[1] = (char)(0x80 | ((c >> 6) & 0x3F));
        out[2] = (char)(0x80 | (c & 0x3F));
        return 3;
    }
    out[0] = (char)(0xF0 | (c >> 18));
    out[1] = (char)(0x80 | ((c >> 12) & 0x3F));
    out[2] = (char)(0x80 | ((c >> 6) & 0x3F));
    out[3] = (char)(0x80 | (c & 0x3F));
    return 4;
}

int chili_utf8(chili_vm *vm, int32_t code, char out[4])
{
    return utf8_encode(vm, "printc", code, out);
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

/* Nom d'une sorte avec son article, pour les messages d'erreur. */
static const char *tag_name(uint8_t tag)
{
    switch (tag) {
    case CHILI_INT: return "un entier";
    case CHILI_STR: return "une chaine";
    case CHILI_ARR: return "un tableau";
    default:        return "un objet";
    }
}

static chili_value make_ref(uint32_t off, uint8_t tag)
{
    chili_value r;
    r.v = (int32_t)off;
    r.tag = tag;
    return r;
}

/* ------------------------------------------------------------------------ */
/* Tas et ramasse-miettes                                                   */

#define BIT_GET(set, i) (((set)[(i) >> 5] >> ((i) & 31)) & 1u)
#define BIT_SET(set, i) ((set)[(i) >> 5] |= 1u << ((i) & 31))
#define BIT_CLR(set, i) ((set)[(i) >> 5] &= ~(1u << ((i) & 31)))

#define BLK_SIZE(h) ((h) & 0xFFFFu)
#define BLK_KIND(h) ((h) >> 16)
#define MAX_BLOCK 0xFFFFu

static uint8_t htag(const chili_vm *vm, uint32_t o)
{
    return (uint8_t)((vm->heap_tags[o >> 2] >> ((o & 3) * 2)) & 3u);
}

static void set_htag(chili_vm *vm, uint32_t o, uint8_t tag)
{
    uint8_t shift = (uint8_t)((o & 3) * 2);
    vm->heap_tags[o >> 2] = (uint8_t)((vm->heap_tags[o >> 2] & ~(3u << shift)) | ((unsigned)tag << shift));
}

static chili_value heap_get(const chili_vm *vm, uint32_t o)
{
    chili_value r;
    r.v = (int32_t)vm->heap[o];
    r.tag = htag(vm, o);
    return r;
}

static void heap_set(chili_vm *vm, uint32_t o, chili_value v)
{
    vm->heap[o] = (uint32_t)v.v;
    set_htag(vm, o, v.tag);
}

static void heap_init(chili_vm *vm)
{
    memset(vm->heap_starts, 0, sizeof vm->heap_starts);
    memset(vm->heap_marks, 0, sizeof vm->heap_marks);
    memset(vm->heap_tags, 0, sizeof vm->heap_tags);
    vm->heap[0] = CHILI_HEAP_WORDS; /* un seul bloc libre */
    BIT_SET(vm->heap_starts, 0);
    vm->gc_runs = 0;
}

/* Une valeur désigne-t-elle un bloc du tas ? Si oui, son offset dans *off. */
static int heap_ref(chili_value v, uint32_t *off)
{
    if (v.tag == CHILI_INT || (v.tag == CHILI_STR && ((uint32_t)v.v & CHILI_STR_CONST)))
        return 0;
    *off = (uint32_t)v.v;
    return 1;
}

static void mark_value(chili_vm *vm, chili_value v, int *changed)
{
    uint32_t o;
    if (heap_ref(v, &o) && !BIT_GET(vm->heap_marks, o)) {
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

    /* Propagation : champs et cases des blocs marqués, jusqu'à stabilité.
     * Le contenu d'une chaîne n'est jamais parcouru. */
    while (changed) {
        changed = 0;
        for (uint32_t o = 0; o < CHILI_HEAP_WORDS; o += BLK_SIZE(vm->heap[o])) {
            uint32_t h = vm->heap[o];
            uint32_t kind = BLK_KIND(h);
            if (kind != KIND_FREE && kind != KIND_STR && BIT_GET(vm->heap_marks, o))
                for (uint32_t f = 1; f < BLK_SIZE(h); f++)
                    mark_value(vm, heap_get(vm, o + f), &changed);
        }
    }

    /* Balayage : libère les blocs non marqués et fusionne les blocs libres voisins. */
    uint32_t prev_free = CHILI_HEAP_WORDS; /* aucun */
    for (uint32_t o = 0; o < CHILI_HEAP_WORDS;) {
        uint32_t size = BLK_SIZE(vm->heap[o]);
        if (BLK_KIND(vm->heap[o]) != KIND_FREE && !BIT_GET(vm->heap_marks, o))
            vm->heap[o] = size; /* libre */
        if (BLK_KIND(vm->heap[o]) == KIND_FREE) {
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
        if (BLK_KIND(vm->heap[o]) == KIND_FREE && BLK_SIZE(vm->heap[o]) >= need)
            return o;
    return CHILI_HEAP_WORDS;
}

/*
 * Alloue un bloc de need mots (en-tête compris) du genre kind, contenu à 0
 * (entiers). Renvoie son offset, ou CHILI_HEAP_WORDS si la mémoire est pleine.
 * Peut lancer le ramasse-miettes : toute valeur vivante doit être sur la pile.
 */
static uint32_t heap_alloc(chili_vm *vm, uint32_t need, uint32_t kind)
{
    if (need > MAX_BLOCK || need > CHILI_HEAP_WORDS)
        return CHILI_HEAP_WORDS;
    uint32_t o = find_free(vm, need);
    if (o == CHILI_HEAP_WORDS) {
        gc(vm);
        o = find_free(vm, need);
        if (o == CHILI_HEAP_WORDS)
            return CHILI_HEAP_WORDS;
    }
    uint32_t size = BLK_SIZE(vm->heap[o]);
    if (size > need) { /* découpe : le reste forme un bloc libre */
        vm->heap[o + need] = size - need;
        BIT_SET(vm->heap_starts, o + need);
        size = need;
    }
    vm->heap[o] = size | (kind << 16);
    for (uint32_t i = 1; i < size; i++)
        heap_set(vm, o + i, chili_int(0));
    return o;
}

const uint8_t *chili_str(const chili_vm *vm, chili_value v, uint32_t *len)
{
    *len = 0;
    if (v.tag != CHILI_STR)
        return NULL;
    uint32_t u = (uint32_t)v.v;
    if (u & CHILI_STR_CONST) {
        const uint8_t *p = vm->strings[u & ~CHILI_STR_CONST];
        *len = rd16(p);
        return p + 2;
    }
    *len = vm->heap[u + 1];
    return (const uint8_t *)&vm->heap[u + 2];
}

chili_status chili_new_str(chili_vm *vm, const uint8_t *bytes, uint32_t len, chili_value *out)
{
    uint32_t need = 2 + (len + 3) / 4;
    uint32_t o = (len > MAX_BLOCK * 4u) ? CHILI_HEAP_WORDS : heap_alloc(vm, need, KIND_STR);
    if (o == CHILI_HEAP_WORDS)
        return chili_fail(vm, "memoire des objets pleine (%u mots)", (unsigned)CHILI_HEAP_WORDS);
    vm->heap[o + 1] = len;
    if (bytes)
        memcpy(&vm->heap[o + 2], bytes, len);
    *out = make_ref(o, CHILI_STR);
    return CHILI_OK;
}

const char *chili_text(const chili_vm *vm, chili_value v, char *buf, size_t size, uint32_t *len)
{
    int n = 0;
    switch (v.tag) {
    case CHILI_INT:
        n = snprintf(buf, size, "%ld", (long)v.v);
        break;
    case CHILI_STR:
        return (const char *)chili_str(vm, v, len);
    case CHILI_ARR:
        n = snprintf(buf, size, "<tableau %u>", (unsigned)(BLK_SIZE(vm->heap[v.v]) - 1));
        break;
    default: {
        int cls_len;
        uint32_t c = BLK_KIND(vm->heap[v.v]) - 1;
        const char *cls = name_of(vm, vm->classes[c].name, &cls_len);
        n = snprintf(buf, size, "<%.*s>", cls_len, cls);
        break;
    }
    }
    if (n < 0)
        n = 0;
    if ((size_t)n >= size)
        n = (int)size - 1;
    *len = (uint32_t)n;
    return buf;
}

/* ------------------------------------------------------------------------ */
/* Natives de chaînes (même comportement que compiler/natives)              */

chili_status chili_native_str_sub(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    uint32_t len;
    chili_str(vm, args[0], &len);
    int64_t a = args[1].v, b = args[2].v;
    if (a < 0) a = 0;
    if (b < 0) b = 0;
    if (a > (int64_t)len) a = len;
    if (b > (int64_t)len) b = len;
    if (a >= b)
        return chili_new_str(vm, NULL, 0, ret);
    if (chili_new_str(vm, NULL, (uint32_t)(b - a), ret) != CHILI_OK)
        return CHILI_ERROR;
    /* Après l'allocation : le tas ne déplace jamais un bloc, mais on relit par prudence. */
    const uint8_t *s = chili_str(vm, args[0], &len);
    memcpy(&vm->heap[ret->v + 2], s + a, (size_t)(b - a));
    return CHILI_OK;
}

chili_status chili_native_str_find(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    uint32_t n, m;
    const uint8_t *s = chili_str(vm, args[0], &n);
    const uint8_t *p = chili_str(vm, args[1], &m);
    *ret = chili_int(-1);
    if (m > n)
        return CHILI_OK;
    for (uint32_t i = 0; i + m <= n; i++) {
        if (memcmp(s + i, p, m) == 0) {
            *ret = chili_int((int32_t)i);
            break;
        }
    }
    return CHILI_OK;
}

chili_status chili_native_str_char(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    char buf[4];
    int n = utf8_encode(vm, "str.char", args[0].v, buf);
    if (n == 0)
        return CHILI_ERROR;
    return chili_new_str(vm, (const uint8_t *)buf, (uint32_t)n, ret);
}

chili_status chili_native_str_int(chili_vm *vm, const chili_value *args, chili_value *ret)
{
    uint32_t n;
    const uint8_t *s = chili_str(vm, args[0], &n);
    uint32_t i = 0, end = n, v = 0;
    int neg = 0;
    *ret = chili_int(0);
    while (i < end && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n'))
        i++;
    while (end > i && (s[end - 1] == ' ' || s[end - 1] == '\t' || s[end - 1] == '\r' || s[end - 1] == '\n'))
        end--;
    if (i < end && (s[i] == '-' || s[i] == '+'))
        neg = s[i++] == '-';
    if (i == end)
        return CHILI_OK;
    for (; i < end; i++) {
        if (s[i] < '0' || s[i] > '9')
            return CHILI_OK;
        v = v * 10u + (uint32_t)(s[i] - '0');
    }
    *ret = chili_int((int32_t)(neg ? 0u - v : v));
    return CHILI_OK;
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
        case OP_PUSHS:
            if (rd16(a) >= vm->num_strings)
                return chili_fail(vm, "f%u : chaine %u inexistante", fi, rd16(a));
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
    uint16_t num_strings = rd16(data + 20);
    if (num_funcs == 0 || num_funcs > CHILI_MAX_FUNCS)
        return chili_fail(vm, "nombre de fonctions invalide (%u, max %u)", num_funcs, CHILI_MAX_FUNCS);
    if (num_classes > CHILI_MAX_CLASSES)
        return chili_fail(vm, "trop de classes (%u, max %u)", num_classes, CHILI_MAX_CLASSES);
    if (num_names > CHILI_MAX_NAMES)
        return chili_fail(vm, "trop de noms (%u, max %u)", num_names, CHILI_MAX_NAMES);
    if (num_strings > CHILI_MAX_STRINGS)
        return chili_fail(vm, "trop de chaines constantes (%u, max %u)", num_strings, CHILI_MAX_STRINGS);

    vm->num_globals = rd16(data + 6);
    vm->num_funcs = num_funcs;
    vm->entry = rd16(data + 10);
    vm->num_classes = num_classes;
    vm->num_names = num_names;
    vm->num_strings = num_strings;

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
    for (uint16_t i = 0; i < num_strings; i++) {
        NEED(2);
        vm->strings[i] = data + off;
        NEED(2 + (size_t)rd16(data + off));
        off += 2 + (size_t)rd16(data + off);
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
        vm->stack[vm->sp++] = chili_int(0);
    chili_frame *fr = &vm->frames[vm->num_frames++];
    fr->fn = fn;
    fr->pc = f->offset;
    fr->base = (uint16_t)base;
    return CHILI_OK;
}

chili_status chili_reset(chili_vm *vm)
{
    for (uint32_t i = 0; i < CHILI_STACK_SIZE; i++)
        vm->stack[i] = chili_int(0);
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

static int truthy(chili_value v) { return v.tag != CHILI_INT || v.v != 0; }

/* Compare deux chaînes octet par octet : <0, 0 ou >0. */
static int str_cmp(const chili_vm *vm, chili_value a, chili_value b)
{
    uint32_t la, lb;
    const uint8_t *pa = chili_str(vm, a, &la);
    const uint8_t *pb = chili_str(vm, b, &lb);
    int c = memcmp(pa, pb, la < lb ? la : lb);
    if (c != 0)
        return c;
    return (la > lb) - (la < lb);
}

static int values_equal(const chili_vm *vm, chili_value a, chili_value b)
{
    if (a.tag != b.tag)
        return 0;
    if (a.tag == CHILI_STR)
        return str_cmp(vm, a, b) == 0;
    return a.v == b.v;
}

/* Opérateur du source correspondant à un opcode, pour les messages. */
static const char *op_symbol(uint8_t op)
{
    switch (op) {
    case OP_ADD: return "+";
    case OP_SUB: case OP_NEG: return "-";
    case OP_MUL: return "*";
    case OP_DIV: return "/";
    case OP_MOD: return "%";
    case OP_AND: return "&";
    case OP_OR: return "|";
    case OP_XOR: return "^";
    case OP_SHL: return "<<";
    case OP_SHR: return ">>";
    case OP_BNOT: return "~";
    case OP_LT: return "<";
    case OP_LE: return "<=";
    case OP_GT: return ">";
    default: return ">=";
    }
}

/* Longueur d'un tableau ou d'une chaîne ; 0 et *ok = 0 pour une autre sorte. */
static uint32_t length_of(const chili_vm *vm, chili_value x, int *ok)
{
    uint32_t n = 0;
    *ok = 1;
    if (x.tag == CHILI_ARR)
        return BLK_SIZE(vm->heap[x.v]) - 1;
    if (x.tag == CHILI_STR) {
        chili_str(vm, x, &n);
        return n;
    }
    *ok = 0;
    return 0;
}

#define PUSH(x) (vm->stack[vm->sp++] = (x))
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
        char msg_[112]; \
        int fn_len_; \
        const char *fn_name_ = name_of(vm, vm->funcs[fr->fn].name, &fn_len_); \
        snprintf(msg_, sizeof msg_, __VA_ARGS__); \
        return chili_fail(vm, "%s (dans %.*s, pc %04lx)", msg_, fn_len_, fn_name_, (unsigned long)pc); \
    } while (0)
#define NEED_INTS(x, y) do { \
        if ((x).tag != CHILI_INT || (y).tag != CHILI_INT) \
            FAIL("'%s' impossible entre %s et %s", op_symbol(op), tag_name((x).tag), tag_name((y).tag)); \
    } while (0)
#define FAIL_FULL() FAIL("memoire des objets pleine (%u mots)", (unsigned)CHILI_HEAP_WORDS)

        switch (op) {
        case OP_HALT:
            vm->state = CHILI_HALTED;
            return CHILI_HALTED;

        case OP_PUSH:  PUSH(chili_int((int32_t)rd32(a))); break;
        case OP_PUSH8: PUSH(chili_int((int8_t)a[0])); break;
        case OP_PUSHS: {
            chili_value s;
            s.v = (int32_t)(CHILI_STR_CONST | rd16(a));
            s.tag = CHILI_STR;
            PUSH(s);
            break;
        }
        case OP_POP:   vm->sp--; break;
        case OP_DUP2: {
            chili_value x = vm->stack[vm->sp - 2], y = vm->stack[vm->sp - 1];
            PUSH(x);
            PUSH(y);
            break;
        }

        case OP_LOAD:   PUSH(vm->stack[fr->base + a[0]]); break;
        case OP_STORE:  vm->stack[fr->base + a[0]] = POP(); break;
        case OP_GLOAD:  PUSH(vm->stack[rd16(a)]); break;
        case OP_GSTORE: vm->stack[rd16(a)] = POP(); break;

        case OP_ADD: {
            chili_value x = vm->stack[vm->sp - 2], y = vm->stack[vm->sp - 1];
            if (x.tag == CHILI_INT && y.tag == CHILI_INT) {
                vm->sp--;
                /* Arithmétique modulo 2^32, en non signé (pas de comportement indéfini). */
                TOP.v = (int32_t)((uint32_t)x.v + (uint32_t)y.v);
                break;
            }
            if ((x.tag != CHILI_INT && x.tag != CHILI_STR) || (y.tag != CHILI_INT && y.tag != CHILI_STR))
                FAIL("'+' impossible entre %s et %s", tag_name(x.tag), tag_name(y.tag));
            /* Concaténation : les opérandes restent sur la pile pendant l'allocation. */
            char bx[16], by[16];
            uint32_t lx, ly;
            chili_text(vm, x, bx, sizeof bx, &lx);
            chili_text(vm, y, by, sizeof by, &ly);
            chili_value r;
            if (chili_new_str(vm, NULL, lx + ly, &r) != CHILI_OK)
                FAIL_FULL();
            const char *px = chili_text(vm, x, bx, sizeof bx, &lx);
            const char *py = chili_text(vm, y, by, sizeof by, &ly);
            uint8_t *dst = (uint8_t *)&vm->heap[r.v + 2];
            memcpy(dst, px, lx);
            memcpy(dst + lx, py, ly);
            vm->sp -= 2;
            PUSH(r);
            break;
        }
        case OP_SUB: { chili_value y = POP(); NEED_INTS(TOP, y); TOP.v = (int32_t)((uint32_t)TOP.v - (uint32_t)y.v); break; }
        case OP_MUL: { chili_value y = POP(); NEED_INTS(TOP, y); TOP.v = (int32_t)((uint32_t)TOP.v * (uint32_t)y.v); break; }
        case OP_DIV: case OP_MOD: {
            chili_value y = POP();
            NEED_INTS(TOP, y);
            int32_t b = y.v, x = TOP.v;
            if (b == 0)
                FAIL("%s", op == OP_DIV ? "division par zero" : "modulo par zero");
            if (x == INT32_MIN && b == -1)
                TOP.v = (op == OP_DIV) ? INT32_MIN : 0;
            else
                TOP.v = (op == OP_DIV) ? x / b : x % b;
            break;
        }
        case OP_AND: { chili_value y = POP(); NEED_INTS(TOP, y); TOP.v &= y.v; break; }
        case OP_OR:  { chili_value y = POP(); NEED_INTS(TOP, y); TOP.v |= y.v; break; }
        case OP_XOR: { chili_value y = POP(); NEED_INTS(TOP, y); TOP.v ^= y.v; break; }
        case OP_SHL: {
            chili_value y = POP();
            NEED_INTS(TOP, y);
            TOP.v = (int32_t)((uint32_t)TOP.v << ((uint32_t)y.v & 31u));
            break;
        }
        case OP_SHR: {
            /* Décalage arithmétique, sans dépendre du comportement du compilateur. */
            chili_value y = POP();
            NEED_INTS(TOP, y);
            uint32_t b = (uint32_t)y.v & 31u;
            int32_t x = TOP.v;
            TOP.v = (x >= 0) ? (int32_t)((uint32_t)x >> b) : (int32_t)~(~(uint32_t)x >> b);
            break;
        }
        case OP_NEG: case OP_BNOT:
            if (TOP.tag != CHILI_INT)
                FAIL("'%s' impossible sur %s", op_symbol(op), tag_name(TOP.tag));
            TOP.v = (op == OP_NEG) ? (int32_t)(0u - (uint32_t)TOP.v) : ~TOP.v;
            break;
        case OP_NOT: TOP = chili_int(!truthy(TOP)); break;

        case OP_EQ: case OP_NE: {
            chili_value y = POP();
            int eq = values_equal(vm, TOP, y);
            TOP = chili_int(op == OP_EQ ? eq : !eq);
            break;
        }
        case OP_LT: case OP_LE: case OP_GT: case OP_GE: {
            chili_value y = POP(), x = TOP;
            int c;
            if (x.tag == CHILI_INT && y.tag == CHILI_INT)
                c = (x.v > y.v) - (x.v < y.v);
            else if (x.tag == CHILI_STR && y.tag == CHILI_STR)
                c = str_cmp(vm, x, y);
            else
                FAIL("'%s' impossible entre %s et %s", op_symbol(op), tag_name(x.tag), tag_name(y.tag));
            int r = (op == OP_LT) ? c < 0 : (op == OP_LE) ? c <= 0 : (op == OP_GT) ? c > 0 : c >= 0;
            TOP = chili_int(r);
            break;
        }

        case OP_JMP: fr->pc += (int16_t)rd16(a); break;
        case OP_JZ:  if (!truthy(POP())) fr->pc += (int16_t)rd16(a); break;
        case OP_JNZ: if (truthy(POP())) fr->pc += (int16_t)rd16(a); break;

        case OP_CALL:
            if (enter(vm, rd16(a)) != CHILI_OK) {
                char msg[sizeof vm->error];
                memcpy(msg, vm->error, sizeof msg);
                FAIL("%.100s", msg);
            }
            break;

        case OP_CALL_NATIVE: {
            const chili_native *nat = &vm->natives[rd16(a)];
            uint8_t argc = a[2];
            const chili_value *args = &vm->stack[vm->sp - argc];
            for (uint8_t i = 0; i < argc && nat->sig && nat->sig[i]; i++) {
                char want = nat->sig[i];
                if ((want == 'i' && args[i].tag != CHILI_INT) || (want == 's' && args[i].tag != CHILI_STR))
                    FAIL("%s : l'argument %u doit etre %s (recu : %s)", nat->name, (unsigned)(i + 1),
                         want == 'i' ? "un entier" : "une chaine", tag_name(args[i].tag));
            }
            /* Les arguments restent sur la pile pendant l'appel (ramasse-miettes). */
            chili_value ret = chili_int(0);
            chili_status st = nat->fn(vm, args, &ret);
            vm->sp -= argc;
            PUSH(ret);
            if (st == CHILI_ERROR)
                return CHILI_ERROR;
            if (st == CHILI_YIELD)
                return CHILI_YIELD;
            break;
        }

        case OP_RET: {
            chili_value r = POP();
            vm->sp = fr->base;
            if (--vm->num_frames == 0) {
                vm->state = CHILI_HALTED;
                return CHILI_HALTED;
            }
            PUSH(r);
            break;
        }

        case OP_ALLOC: {
            uint16_t c = rd16(a);
            uint32_t o = heap_alloc(vm, (uint32_t)vm->classes[c].num_fields + 1, (uint32_t)c + 1);
            if (o == CHILI_HEAP_WORDS)
                FAIL_FULL();
            PUSH(make_ref(o, CHILI_OBJ));
            break;
        }

        case OP_GETF: case OP_SETF: {
            chili_value val = chili_int(0);
            if (op == OP_SETF)
                val = POP();
            chili_value obj = POP();
            if (obj.tag != CHILI_OBJ || a[0] + 1u >= BLK_SIZE(vm->heap[obj.v]))
                FAIL("acces a un champ sur une valeur qui n'est pas un objet");
            if (op == OP_SETF)
                heap_set(vm, (uint32_t)obj.v + 1 + a[0], val);
            else
                PUSH(heap_get(vm, (uint32_t)obj.v + 1 + a[0]));
            break;
        }

        case OP_CALLM: {
            uint16_t sel = rd16(a), fn;
            uint8_t argc = a[2];
            int sel_len, cls_len;
            const char *sel_name = name_of(vm, sel, &sel_len);
            chili_value obj = vm->stack[vm->sp - argc - 1];
            if (obj.tag != CHILI_OBJ)
                FAIL("appel de %.*s() sur une valeur qui n'est pas un objet", sel_len, sel_name);
            uint16_t c = (uint16_t)(BLK_KIND(vm->heap[obj.v]) - 1);
            const char *cls_name = name_of(vm, vm->classes[c].name, &cls_len);
            if (!find_method(vm, c, sel, &fn))
                FAIL("la classe %.*s n'a pas de methode %.*s", cls_len, cls_name, sel_len, sel_name);
            if (vm->funcs[fn].num_params != argc + 1u)
                FAIL("%.*s.%.*s attend %u argument(s), %u fourni(s)", cls_len, cls_name, sel_len, sel_name,
                     (unsigned)(vm->funcs[fn].num_params - 1), (unsigned)argc);
            if (enter(vm, fn) != CHILI_OK) {
                char msg[sizeof vm->error];
                memcpy(msg, vm->error, sizeof msg);
                FAIL("%.100s", msg);
            }
            break;
        }

        case OP_NEWARR: {
            chili_value n = TOP;
            if (n.tag != CHILI_INT)
                FAIL("array() attend un entier (recu : %s)", tag_name(n.tag));
            if (n.v < 0)
                FAIL("array(%ld) : taille invalide", (long)n.v);
            uint32_t o = heap_alloc(vm, (uint32_t)n.v + 1, KIND_ARR);
            if (o == CHILI_HEAP_WORDS)
                FAIL_FULL();
            TOP = make_ref(o, CHILI_ARR);
            break;
        }

        case OP_MKARR: {
            uint16_t n = rd16(a);
            /* Les éléments restent sur la pile pendant l'allocation. */
            uint32_t o = heap_alloc(vm, (uint32_t)n + 1, KIND_ARR);
            if (o == CHILI_HEAP_WORDS)
                FAIL_FULL();
            vm->sp -= n;
            for (uint16_t i = 0; i < n; i++)
                heap_set(vm, o + 1 + i, vm->stack[vm->sp + i]);
            PUSH(make_ref(o, CHILI_ARR));
            break;
        }

        case OP_GETI: case OP_SETI: {
            chili_value val = chili_int(0);
            if (op == OP_SETI)
                val = POP();
            chili_value i = POP(), x = POP();
            int ok;
            if (op == OP_SETI && x.tag == CHILI_STR)
                FAIL("les chaines ne sont pas modifiables");
            uint32_t n = length_of(vm, x, &ok);
            if (!ok)
                FAIL("indexation impossible sur %s", tag_name(x.tag));
            if (i.tag != CHILI_INT)
                FAIL("l'indice doit etre un entier (recu : %s)", tag_name(i.tag));
            if (i.v < 0 || (uint32_t)i.v >= n)
                FAIL("indice %ld hors limites (taille %lu)", (long)i.v, (unsigned long)n);
            if (x.tag == CHILI_STR) {
                uint32_t len;
                PUSH(chili_int(chili_str(vm, x, &len)[i.v]));
            } else if (op == OP_GETI) {
                PUSH(heap_get(vm, (uint32_t)x.v + 1 + (uint32_t)i.v));
            } else {
                heap_set(vm, (uint32_t)x.v + 1 + (uint32_t)i.v, val);
            }
            break;
        }

        case OP_LEN: {
            int ok;
            uint32_t n = length_of(vm, TOP, &ok);
            if (!ok)
                FAIL("len() impossible sur %s", tag_name(TOP.tag));
            TOP = chili_int((int32_t)n);
            break;
        }

        default:
            FAIL("opcode inconnu");
        }
#undef FAIL
#undef NEED_INTS
#undef FAIL_FULL
    }
}

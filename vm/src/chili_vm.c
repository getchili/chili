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
    OP_CALL = 0x50, OP_CALL_NATIVE = 0x51, OP_RET = 0x52
};

#define HEADER_SIZE 16
#define FUNC_SIZE 8

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
    case OP_PUSH8: case OP_LOAD: case OP_STORE:
        return 2;
    case OP_GLOAD: case OP_GSTORE: case OP_JMP: case OP_JZ: case OP_JNZ: case OP_CALL:
        return 3;
    case OP_CALL_NATIVE:
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
        return chili_fail(vm, "version de bytecode %u non supportee (attendu %u)",
                          data[4], CHILI_BYTECODE_VERSION);

    uint16_t num_funcs = rd16(data + 8);
    uint32_t code_size = rd32(data + 12);
    size_t total = HEADER_SIZE + (size_t)num_funcs * FUNC_SIZE + (size_t)code_size + 4;
    if (code_size > size || total != size)
        return chili_fail(vm, "taille incoherente (%lu octets, attendu %lu)",
                          (unsigned long)size, (unsigned long)total);
    if (chili_crc32(data, size - 4) != rd32(data + size - 4))
        return chili_fail(vm, "CRC invalide : fichier corrompu");
    if (num_funcs == 0 || num_funcs > CHILI_MAX_FUNCS)
        return chili_fail(vm, "nombre de fonctions invalide (%u, max %u)", num_funcs, CHILI_MAX_FUNCS);

    vm->num_globals = rd16(data + 6);
    vm->num_funcs = num_funcs;
    vm->entry = rd16(data + 10);
    vm->code = data + HEADER_SIZE + (size_t)num_funcs * FUNC_SIZE;
    vm->code_size = code_size;

    if (vm->entry >= num_funcs)
        return chili_fail(vm, "fonction d'entree inexistante");
    if (vm->num_globals >= CHILI_STACK_SIZE)
        return chili_fail(vm, "trop de globales pour la pile (%u)", vm->num_globals);

    for (uint16_t i = 0; i < num_funcs; i++) {
        const uint8_t *e = data + HEADER_SIZE + (size_t)i * FUNC_SIZE;
        chili_func *f = &vm->funcs[i];
        f->offset = rd32(e);
        f->num_params = e[4];
        f->num_locals = e[5];
        f->max_stack = rd16(e + 6);
    }
    for (uint16_t i = 0; i < num_funcs; i++) {
        chili_func *f = &vm->funcs[i];
        f->end = (i + 1 < num_funcs) ? vm->funcs[i + 1].offset : code_size;
        if ((i == 0 && f->offset != 0) || f->offset >= f->end || f->end > code_size)
            return chili_fail(vm, "f%u : bornes de code invalides", i);
        if (f->num_params > f->num_locals)
            return chili_fail(vm, "f%u : plus de parametres que de locales", i);
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
    vm->sp = vm->num_globals;
    vm->num_frames = 0;
    vm->error[0] = '\0';
    vm->state = CHILI_OK;
    if (enter(vm, vm->entry) != CHILI_OK)
        return CHILI_ERROR;
    return CHILI_OK;
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

#define FAIL(msg) chili_fail(vm, "%s (fonction f%u, pc %04lx)", msg, fr->fn, (unsigned long)pc)

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
                return FAIL(op == OP_DIV ? "division par zero" : "modulo par zero");
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
                return FAIL(msg);
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

        default:
            return FAIL("opcode inconnu");
        }
#undef FAIL
    }
}

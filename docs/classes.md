# Classes en Chili — version 1

Ajout au langage de base (`spec.md`). Nécessite le bytecode v2 et le firmware 0.2.0.

## 1. Exemple

```chili
abstract class Animal {
    let pattes = 4

    constructor(p) {
        this.pattes = p
    }

    abstract function cri()

    function parle() {
        print(this.cri())
    }
}

class Chien extends Animal {
    constructor() {
        super(4)
    }

    function cri() {
        return 1
    }
}

class Main {
    function main() {
        let c = new Chien()
        c.parle()
    }
}
```

## 2. Mots-clés

`class`, `abstract`, `extends`, `constructor`, `this`, `super`, `new`, `function`.
`fn` n'existe plus : toutes les fonctions se déclarent avec `function`.

## 3. Programme et point d'entrée

Au niveau global : des classes, des variables globales (`let`) et des fonctions (`function`).

Le programme doit définir **`class Main`** avec une méthode **`main()`** sans paramètre.
Au démarrage, après l'initialisation des globales, la VM crée un objet `Main`
(`new Main()`, donc son constructeur ne doit pas avoir de paramètre) puis appelle sa méthode `main()`.

## 4. Déclarer une classe

```ebnf
classDecl   = [ "abstract" ] "class" IDENT [ "extends" IDENT ] "{" { member } "}" ;
member      = field | constructor | method ;
field       = "let" IDENT [ "=" expr ] ;                  (* 0 par défaut *)
constructor = "constructor" "(" [ params ] ")" block ;
method      = [ "abstract" ] "function" IDENT "(" [ params ] ")" ( block | (* rien si abstract *) ) ;
```

- **Champs** : `let nom = valeur` (sans valeur : 0). Initialisés à la création de l'objet,
  ceux de la classe parente d'abord, avant l'appel du constructeur. Les initialiseurs
  peuvent utiliser les globales et les fonctions, mais pas `this`.
- **Les champs sont privés** : on n'y accède que par `this.nom`, dans les méthodes et le
  constructeur de la classe et de ses classes filles. De l'extérieur, on passe par des méthodes.
- **Constructeur** : au plus un par classe. Une classe sans constructeur hérite de celui
  de sa classe parente (mêmes paramètres) ; sans parent, son constructeur ne prend rien.
- Si la classe parente a un constructeur avec des paramètres, le constructeur de la classe fille
  doit appeler `super(...)`.

## 5. Héritage et classes abstraites

- Héritage simple : `class B extends A`. Les classes peuvent être déclarées dans n'importe quel ordre ;
  un cycle (`A extends B`, `B extends A`) est une erreur.
- Une classe fille hérite des champs et des méthodes. Elle ne peut pas redéclarer un champ hérité.
- **Redéfinition** : une méthode de même nom remplace celle du parent ; elle doit avoir le même
  nombre de paramètres.
- `super.methode(args)` appelle la version de la classe parente.
- **`abstract function nom(params)`** : méthode sans corps, seulement dans une `abstract class`.
- Une **`abstract class`** ne peut pas être créée avec `new`.
- Une classe non abstraite doit définir toutes les méthodes abstraites qu'elle hérite.

## 6. Utiliser des objets

| Écriture              | Sens                                                        |
|-----------------------|-------------------------------------------------------------|
| `new Chien(a, b)`     | crée un objet et appelle son constructeur                   |
| `obj.methode(a, b)`   | appel de méthode, choisi selon la classe réelle de `obj`    |
| `this.champ`          | lecture d'un champ (dans une méthode / un constructeur)     |
| `this.champ = v`      | écriture d'un champ (`+=`, `-=`… aussi)                    |
| `this.methode()`      | appel d'une méthode sur l'objet courant                     |
| `super(a)` / `super.m()` | constructeur / méthode de la classe parente             |
| `a == b`              | vrai si `a` et `b` désignent le même objet                  |

- Un objet est une valeur comme une autre : on le range dans une variable, un champ,
  on le passe en paramètre ou on le renvoie.
- `0` sert de « pas d'objet ». Appeler une méthode sur une valeur qui n'est pas un objet
  est une erreur d'exécution.
- Les objets inutilisés sont récupérés automatiquement (ramasse-miettes) quand la mémoire
  des objets est pleine.
- Dans une méthode, un appel sans `this.` (`f()`) désigne une **fonction globale**.

## 7. Bytecode (v2)

Voir `bytecode.md` §5 : table des classes, opcodes `ALLOC`, `GETF`, `SETF`, `CALLM`,
noms des classes et méthodes pour les messages d'erreur.

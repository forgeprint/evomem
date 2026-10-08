# Interface placement

Go interfaces are satisfied implicitly, which moves the question of who owns
an interface from the implementer to the caller. Getting this backwards is the
most common way Java- or C#-shaped design enters a Go codebase.

| #   | Check                                                                                   | How                                                                        | Source                                     |
| --- | --------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------ |
| IP1 | Interfaces are declared in the package that calls them                                  | for each `interface {`, find the callers; they are in the same package     | Go Code Review Comments — interfaces       |
| IP2 | Constructors return concrete types                                                      | `grep -rn "^func New" --include=*.go`; the return type is a struct pointer | Go Code Review Comments — interfaces       |
| IP3 | No interface exists only to be mocked, beside its single implementation                 | interfaces with one implementation in the same package                     | Go Code Review Comments — interfaces       |
| IP4 | Consumer interfaces are small: the methods that consumer calls, and no more             | count methods; compare with the call sites                                 | Effective Go — interfaces; Go proverbs     |
| IP5 | Compile-time assertions (`var _ I = (*T)(nil)`) live where both sides are visible       | in the wiring package or a test                                            | Effective Go — interface checks            |
| IP6 | No `any` / `interface{}` parameter where a type parameter or a named interface would do | grep exported signatures for `any`                                         | Go spec — type parameters; Google Go style |
| IP7 | The domain package's interfaces mention no driver, SQL or HTTP type                     | read the method signatures                                                 | Go Code Review Comments — interfaces       |

## Why each one

**IP1** is the rule that makes Go packages decouple. When the consumer owns a
two-method interface, the producer can add methods, change its internals, or be
replaced, and the consumer never recompiles against anything it does not use.

**IP3** because an interface written "so it can be mocked" beside its only
implementation couples every consumer to every method of it. The test that
needs a fake declares the interface it needs, in the test.

**IP7** because an interface is a boundary only if nothing leaks through its
signature. A domain interface that takes `*sql.Tx` has moved the database into
the domain with an extra layer of indirection.

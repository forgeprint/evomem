# Error wrapping

A Go error is a value, and since Go 1.13 a chain of values. The architecture
question is which errors are part of a package's API and whether callers can
match them without reading strings.

| #   | Check                                                                                              | How                                                                      | Source                                   |
| --- | -------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ---------------------------------------- |
| EW1 | Errors crossing a package boundary are wrapped with `%w` and a short context                       | `grep -rn "fmt.Errorf" --include=*.go`; golangci-lint `errorlint`        | Go blog — Working with errors in Go 1.13 |
| EW2 | Errors are matched with `errors.Is`, `errors.As` or `errors.AsType`, never `==` on a wrapped error | golangci-lint `errorlint`; `go vet` `errorsas`                           | `errors` package; Go 1.26 release notes  |
| EW3 | No decision is made on `err.Error()` text                                                          | `grep -rn "Error()" --include=*.go \| grep -E "Contains\|==\|HasPrefix"` | `errors` package                         |
| EW4 | Sentinel errors a caller may match are exported and documented                                     | `grep -rn "= errors.New" --include=*.go`                                 | Go blog — Working with errors in Go 1.13 |
| EW5 | Wrapping with `%w` is a deliberate API choice; `%v` where the cause must stay private              | read each wrap of a lower layer's error                                  | Go blog — Working with errors in Go 1.13 |
| EW6 | An error is either handled or returned, not logged and returned                                    | read error paths for a log call followed by `return err`                 | Google Go style — logging errors         |
| EW7 | Errors from other modules are wrapped before they leave this module                                | golangci-lint `wrapcheck` on the exported API                            | golangci-lint 2.14 — wrapcheck           |
| EW8 | `panic` does not cross a package API; `recover` exists only at the server boundary                 | `grep -rn "panic(\|recover()" --include=*.go`                            | Effective Go — panic and recover         |

## Why each one

**EW3** is the failure that arrives with a dependency upgrade. An error message
is not an API; the day a driver rewords it, every `strings.Contains` on it
silently takes the other branch.

**EW5** is the one people get backwards. `%w` makes the wrapped error part of
this package's contract — callers can now `errors.Is` against the driver's
sentinel, and the package can never change drivers without breaking them.
Wrapping with `%w` should be a decision, not a habit.

**EW6** because an error logged at every layer on its way up appears four
times in the logs, with four different contexts, and none of them says which
layer decided what to do about it.

# Server and client timeouts

In `net/http`, a zero timeout means no timeout. Every field below starts at
zero, so every one of them is a decision somebody has to write down.

| #    | Check                                                                                                                                 | How                                                                           | Source                                                       |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | ------------------------------------------------------------ |
| ST1  | The server sets `ReadHeaderTimeout`                                                                                                   | read the `http.Server` literal; gosec G112                                    | Go 1.27 — net/http Server; gosec 2.29 — G112                 |
| ST2  | The server sets `ReadTimeout`, `WriteTimeout` and `IdleTimeout`                                                                       | read the `http.Server` literal                                                | Go 1.27 — net/http Server                                    |
| ST3  | No `http.ListenAndServe` or `http.Serve` with the package-level server                                                                | gosec G114                                                                    | gosec 2.29 — G114                                            |
| ST4  | No `http.Get`, `http.Post` or `http.DefaultClient`; one shared `http.Client` has `Timeout` set                                        | grep all four; read the client construction                                   | Go 1.27 — net/http Client                                    |
| ST5  | Every response body is closed                                                                                                         | golangci-lint `bodyclose`                                                     | golangci-lint 2.14 — bodyclose                               |
| ST6  | Request bodies are wrapped in `http.MaxBytesReader` before decoding                                                                   | read each handler that reads the body                                         | Go 1.27 — net/http MaxBytesReader; OWASP API4:2023           |
| ST7  | JSON decoding rejects unknown fields: `Decoder.DisallowUnknownFields()`, or Gin's `binding.EnableDecoderDisallowUnknownFields = true` | grep `json.NewDecoder` and `json.Unmarshal` on request bodies; read Gin setup | Go 1.27 — encoding/json; Gin 1.12 — binding; OWASP API3:2023 |
| ST8  | Decoded structs are validated with bounds on every string and slice                                                                   | read the validation after decoding, or the `binding:` tags in Gin             | validator v10.30; OWASP ASVS 5.0.0 V2                        |
| ST9  | Gin handlers use `ShouldBindJSON`, not `BindJSON`, so the error becomes problem details                                               | grep `BindJSON(` and `Bind(`                                                  | Gin 1.12 — model binding and validation; RFC 9457 §3         |
| ST10 | Structs that are marshalled or unmarshalled carry explicit `json` tags                                                                | golangci-lint `musttag`                                                       | golangci-lint 2.14 — musttag                                 |

## Why each one

**ST1** is the one gosec gives a rule number to. Without a header deadline a
client can hold a connection open by sending one header byte at a time, and a
few hundred such clients exhaust the server's file descriptors.

**ST4** is the client-side twin. `http.DefaultClient` has a zero `Timeout`, so a
downstream that accepts the connection and never answers holds the goroutine,
the connection and whatever the handler had open, indefinitely.

**ST9** is Gin's default working against you. `BindJSON` aborts with a
`text/plain` 400 before your error mapper runs, so clients get two error
formats from one API.

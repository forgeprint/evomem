# Device storage

What a Flutter app keeps on the phone, and what it compiles into the binary,
is readable by whoever holds the phone or the binary, unless the platform
keystore stands in between.

| #   | Check                                                                                                    | How                                                                                              | Source                                           |
| --- | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| DS1 | Tokens, passwords and keys are written with `flutter_secure_storage`, never `shared_preferences`         | `grep -rn "SharedPreferences\|SharedPreferencesAsync" lib`; read every key that is set           | flutter_secure_storage 11.2.0; MASWE-0001        |
| DS2 | Android backup excludes the secure-storage preferences, or backup is off                                 | read `android:allowBackup`, `fullBackupContent` / `dataExtractionRules` in `AndroidManifest.xml` | flutter_secure_storage 11.2.0; MASWE-0006        |
| DS3 | No secret in `--dart-define`, `--dart-define-from-file`, `assets/` or source                             | `grep -rn "String.fromEnvironment" lib`; read what each supplies; grep `assets/` for keys        | Flutter 3.47 — obfuscating Dart code; MASWE-0004 |
| DS4 | Release builds use `--obfuscate --split-debug-info=<dir>`, and the symbol files are archived per release | read the release script                                                                          | Flutter 3.47 — obfuscating Dart code             |
| DS5 | No token or personal data in `print`, `debugPrint` or `log`                                              | `grep -rnE "\b(print\|debugPrint\|log)\(" lib`; `avoid_print` is on                              | MASWE-0005; very_good_analysis 11.0.0            |
| DS6 | Platform channels are generated with Pigeon, or every handler validates its arguments                    | `grep -rn "MethodChannel\|EventChannel" lib android ios`; each without Pigeon is read            | Pigeon 29.0.2; MASVS-PLATFORM-1                  |
| DS7 | Network calls use `https://`; no cleartext exception in the network security config or ATS               | `grep -rn "http://" lib`; read `usesCleartextTraffic` and `NSAppTransportSecurity`               | MASVS-NETWORK-1; MASWE-0026                      |
| DS8 | Sign-out deletes every secure-storage entry the app wrote                                                | read sign-out for `delete` / `deleteAll`                                                         | flutter_secure_storage 11.2.0; MASVS-STORAGE-2   |

## Why each one

**DS3** because `--dart-define` feels like configuration and behaves like
source: the value is a constant in the compiled app. The Flutter obfuscation
page says it plainly — obfuscation renames symbols and encrypts nothing, and
storing secrets in an app is poor practice.

**DS2** is the failure users see first. An Android backup restores the
encrypted preferences onto a device whose keystore never held the key, and the
next read throws `InvalidKeyException`.

**DS6** because a `MethodChannel` handler receives a `dynamic` from the other
side of a language boundary, and the analyzer cannot see across it. Pigeon
generates both ends from one typed definition.

# Durum

Bu dosya **her oturum sonunda üzerine yazılır**: işler şu an nerede, sırada ne
var. Kronolojik kayıt `ilerleme.md`'de; burası anlık görüntü.

Son güncelleme: 2026-10-08 (dördüncü oturum) · Son commit: `a3fe900` · Sürüm: **v0.1.1 yayında**
CI: **tamamı yeşil** (go, crosscheck, flutter)

---

## Fazlar

| Faz | Durum |
| - | - |
| 1 Monorepo + SQLite deposu | tamam |
| 2 MCP sunucusu | tamam |
| 3 Flutter mobil uygulama | **tamam** — iskelet + sqflite + sync + settings + testler |
| 4 Telegram + Jira + HTTP girişi | tamam |
| 5 Sync + arşivleme | tamam |

`plan.md`'deki kutular işaretli; sapmalar o dosyada not düşülmüş ve
gerekçeleri `adr/`'de.

## Ne var

Tek statik Go ikilisi, çalışma zamanı bağımlılığı yok. 11 tane paket:

```
shared/models      Note, açık uçlu source_type, metadata uzatma noktası,
                   elde yazılmış ULID, tainted işareti
shared/database    tek yazıcılı iki havuz, FTS5 arama, delta izleme,
                   tombstone, öneriler, arşivleme
core/mcp           stdio JSON-RPC, spec'e göre yazılmış, iki protokol dönemi
core/api           POST /ingest + Telegram ve Jira webhook'ları
core/sync          Remote arayüzü, worker, PostgreSQL transportu
cmd/evomem         13 alt komut (mcp, serve, pull, restore dahil)
apps/mobile        Flutter 3.47 (Riverpod 3, go_router, sqflite, http, very_good_analysis, l10n)
```

Şema sürümü **3**. Bağımlılıklar: `modernc.org/sqlite`, `jackc/pgx/v5`,
ikisi de vendor'lı ve saf Go.

## Komutlar

```sh
./scripts/ci.sh              # CI'ın çalıştırdığı her şey
./scripts/test-postgres.sh   # sync transportu, tek kullanımlık veritabanıyla
./scripts/build.sh           # dist/evomem

evomem add -project <id> <metin>      # not yaz
evomem search -query <metin>          # ara
evomem review                         # ajanın önerdikleri
evomem review -accept <id>            # kabul et
evomem mcp                            # ajanın başlattığı sunucu
evomem serve                          # HTTP girişi
evomem sync -once                     # aynaya gönder
evomem pull                           # aynadan çek
evomem restore [-confirm]             # aynadan tam geri yükle
evomem sync-status                    # ne bekliyor
evomem archive -dry-run               # ne temizlenecek
```

## Neyin doğrulandığı, neyin doğrulanmadığı

**Gerçekten çalıştığı görüldü:**

- SQLite deposu, FTS5 arama, eşzamanlı yazma — 219 test
- MCP, iki protokol dönemi de, derlenmiş ikiliyle gerçek JSON-RPC satırlarıyla
- HTTP girişinin üç yolu, `curl` ile, kimlik doğrulama hataları dahil
- PostgreSQL transportu, Docker'da gerçek PostgreSQL 17'ye karşı 10 test
- Öneri/inceleme akışı, ajan gözünden ve insan gözünden
- **Flutter mobil**: sqflite persistence, sync, settings — 42 test,
  `flutter analyze` temiz, web release build'i geçiyor

**Hiç denenmedi:**

- Gerçek bir Telegram botu veya gerçek bir Jira instance'ı
- Gerçek bir bulut PostgreSQL'i (`sslmode=require` ile uzak sunucu)

---

## Sırada — [SEN]

Bunlar bende değil, sende:

1. **DCO app + branch koruması** GitHub repo ayarlarından. İmzalar atılıyor
   (`git commit -s`) ama kontrol eden bir şey yok.
2. **Tünel kur** (cloudflared veya ngrok) → Telegram ve Jira webhook'larının
   `evomem serve`'e ulaşması için. `evomem serve` TLS sunmuyor, bilinçli.
3. **Gerçek bot/webhook ile dene.** Telegram `setWebhook`, Jira'da gizli
   anahtarlı webhook. `docs/api.md` ikisinin de adımlarını yazıyor.
4. **Bulut PostgreSQL'i seç** ve `EVOMEM_POSTGRES_DSN` ile dene.
5. ~~İlk sürümü kes.~~ **v0.1.1 yayında.** Sonraki sürüm için tek iş:
   `git tag -a vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z`, sonra taslağı
   oku ve yayınla.

## Sırada — kod

Öncelik sırasına göre, her biri tek oturumluk iş:

1. **Ses dökümü — karar verildi, kod yazılmadı.** ADR-0016 yeniden yazıldı ve
   **accepted**: döküm, evomem'in sahibi olmadığı bir HTTP servisi; yalnızca
   URL biliniyor, yerel yedek yok, `EVOMEM_TRANSCRIPTION_URL` yoksa özellik
   kapalı. Döngüyü yeni bir `evomem transcribe` komutu çalıştırır (`sync`
   kalıbı, launchd/systemd tetikler). Döküm `content`'i ezer ve
   `transcribed: true` ile işaretlenir; Telegram notları zaten tainted.
   Hatalar nota yazılır, `awaiting_transcription` duruyor kalır.
   Yazılacaklar: `core/transcribe` paketi, `evomem transcribe` komutu,
   `EVOMEM_TELEGRAM_BOT_TOKEN` ile `getFile` indirmesi, MCP'de `transcribed`
   işaretinin yüzeye çıkarılması. Mobildeki kayıtlar kapsam dışı — kendi
   ADR'sini gerektirir.
2. **Flutter tarafı ses kaydı → döküm zinciri.** Mobilde kayıt var, dökümü
   tetikleyen bir şey yok; (1) bitmeden anlamı yok.

**Sürüm ve dağıtım tamam** (ADR-0017): `.github/workflows/release.yml` `v*`
tag'inde `scripts/release.sh`'i temiz runner'da çalıştırır, `dist/`'in her
dosyasını — `SHA256SUMS` dahil — attest eder ve release'i taslak açar.
**v0.1.1 kesildi, notları yazıldı ve yayınlandı**; workflow ilk denemede
geçti. İndirilen ikili `gh attestation verify` ile doğrulandı.

Plan'daki (`plan.md`) beş fazın bütün kutuları işaretli. Kalan iş plan dışı:
döküm, dağıtım ve gerçek dünya denemeleri.

## Bilinen sınırlar (hata değil, karar)

- **Türkçe aramada `ı` → `i` katlanmıyor.** `veritabani` yazınca
  `veritabanı` bulunmuyor. ğ/ş/ç/ö/ü katlanıyor. Gövdeleme (stemming) hiç
  yok. Düzeltmesi Türkçe-farkında tokenizer = yeni bağımlılık. → ADR-0003
- **Jira imzasında replay penceresi yok.** Yakalanmış bir teslim, gizli
  anahtar değişene kadar tekrar oynatılabilir. → ADR-0010
- **Yeni bir adaptör `MarkTainted` çağırmayı unutursa** özellik sessizce
  kaybolur. Model paketinden zorlanamıyor; gözden geçirme maddesi. → ADR-0009
- **Arşivleme senkronize edilmemiş notu silmiyor**, yani sync kurulmadan
  `evomem archive` hiçbir şey yapmıyor (ve nedenini söylüyor). → ADR-0012
- **Flutter testleri `databaseFactoryFfiNoIsolate` kullanmak zorunda.**
  Isolate'lı fabrika future'larını gerçek zamanda tamamlar, `testWidgets` ise
  FakeAsync içinde koşar; ikisi karışınca yazma uçarken ağaç atılıyor ve
  sqflite'ın 10 saniyelik lock uyarı timer'ı testi düşürüyor. Yeni bir test
  dosyası `test/sqflite_test_setup.dart`'ı kullanmalı.
- **`evomem mcp -quiet` yalnızca stderr'i susturur**, protokol çıktısını
  değil. stdout protokolün; sunucunun kendisi hakkında söylediği her şey
  stderr'e gider.

## Yeni oturuma nasıl başlanır

1. Bu dosyayı oku.
2. `CLAUDE.md` — mimari kısıtlar ve çalışma kuralları.
3. `ilerleme.md`'nin **son bölümü** — en son ne yapıldı.
4. İlgili `adr/` kaydı — bir karara dokunacaksan önce onu oku; kararlar
   yeniden tartışılmaz, gerekçesiyle değiştirilir.
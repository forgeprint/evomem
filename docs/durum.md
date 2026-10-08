# Durum

Bu dosya **her oturum sonunda üzerine yazılır**: işler şu an nerede, sırada ne
var. Kronolojik kayıt `ilerleme.md`'de; burası anlık görüntü.

Son güncelleme: 2026-10-08 (üçüncü oturum) · Son commit: `52c87ff` sonrası · CI: yeşil (doğrulandı)

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
- **Flutter mobil**: sqflite persistence, sync, settings, testler (core pass)

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
5. **İlk sürümü kes.** Workflow hazır, hiç çalışmadı:

   ```sh
   git push origin main          # workflow GitHub'da olmalı, yoksa tag boşa gider
   git tag -s v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   ```

   Sonra GitHub'da taslağı oku ve yayınla. `gh run watch` ile izlenebilir.

## Sırada — kod

Öncelik sırasına göre, her biri tek oturumluk iş:

1. **Ses dökümü.** Telegram ses mesajları `awaiting_transcription: true` ve
   `telegram_file_id` ile duruyor; `getFile` ile indirip döküme çevirecek
   hiçbir şey yok. ADR-0016 kararı verilmiş (**proposed**, self-hosted
   faster-whisper + yerel Whisper.cpp yedeği) ama tek satır kod yazılmadı.
   `core/api/adapters/transcription` paketi yok.
2. **Flutter tarafı ses kaydı → döküm zinciri.** Mobilde kayıt var, dökümü
   tetikleyen bir şey yok; (1) bitmeden anlamı yok.

**Sürüm ve dağıtım tamam** (ADR-0017): `.github/workflows/release.yml` `v*`
tag'inde `scripts/release.sh`'i temiz runner'da çalıştırır, `dist/`'in her
dosyasını — `SHA256SUMS` dahil — attest eder ve release'i **taslak** olarak
açar. Yayınlamak sende. İlk sürüm henüz kesilmedi ve workflow gerçek bir
tag'le hiç çalışmadı; ilk tag aynı zamanda ilk denemesi olacak.

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
- **Flutter widget testlerinde sqflite_ffi timer cleanup** — test ortamı
  kısıtlama, production kodunda sorun yok (FakeAsync timer cleanup).
- **`evomem mcp -quiet` yalnızca stderr'i susturur**, protokol çıktısını
  değil. stdout protokolün; sunucunun kendisi hakkında söylediği her şey
  stderr'e gider.

## Yeni oturuma nasıl başlanır

1. Bu dosyayı oku.
2. `CLAUDE.md` — mimari kısıtlar ve çalışma kuralları.
3. `ilerleme.md`'nin **son bölümü** — en son ne yapıldı.
4. İlgili `adr/` kaydı — bir karara dokunacaksan önce onu oku; kararlar
   yeniden tartışılmaz, gerekçesiyle değiştirilir.
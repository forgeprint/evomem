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
| 3 Flutter mobil uygulama | kısmen — iskelet + sqflite + sync + settings + testler; **ses kaydı yok** |
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
cmd/evomem         14 alt komut (mcp, serve, pull, restore, transcribe dahil)
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
evomem transcribe -dry-run            # hangi kayıt dökülecek
evomem transcribe                     # kuyruğu işle (servis yoksa kapalı)
```

## Neyin doğrulandığı, neyin doğrulanmadığı

**Gerçekten çalıştığı görüldü:**

- SQLite deposu, FTS5 arama, eşzamanlı yazma — 219 test
- MCP, iki protokol dönemi de, derlenmiş ikiliyle gerçek JSON-RPC satırlarıyla
- HTTP girişinin üç yolu, `curl` ile, kimlik doğrulama hataları dahil
- PostgreSQL transportu, Docker'da gerçek PostgreSQL 17'ye karşı 10 test
- Öneri/inceleme akışı, ajan gözünden ve insan gözünden
- **Ses dökümü (yalnızca Telegram yolu)**: sahte bir OpenAI-uyumlu servis ve sahte bir Bot API ile
  uçtan uca — derlenmiş ikiliyle, gerçek HTTP; `getFile` bir kez gerçek
  Telegram'a da gitti ve `Unauthorized` döndü (hata yolu ve token gizleme
  doğrulandı)
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

1. **Mobilde ses kaydı — yolu kararlaştırıldı, hiçbiri yazılmadı.**
   ADR-0018: telefon sesi `POST /ingest/audio?note=<id>` ile yükler, dosya
   veritabanının yanında durur, `evomem transcribe` onu `Fetcher` dikişinden
   işler. Dört parça iş: (a) mobilde kaydın kendisi — paket, izinler, arayüz
   (ADR-0018 kapsam dışı bıraktı), (b) endpoint ve yerel dosya `Fetcher`'ı,
   (c) telefonun `/ingest`'ten dönen id'yi saklaması, (d) not silinince ses
   dosyasının da silinmesi.

   **Mobilde ses kaydı diye bir şey yok.** `plan.md`'de o kutu yanlış
   işaretliymiş; ADR-0016 de bu yanlışı tekrarlamış. İkisi de düzeltildi.
2. **Push idempotent değil.** Telefon `/ingest`'in döndürdüğü id'yi atıyor ve
   `/ingest` her çağrıda yeni not yaratıyor; yarıda kalan bir batch sonraki
   koşuda notları ikinci kez yazar. (c) ile aynı düzeltme kapatıyor.
3. **Dökümü onaylama akışı.** `tainted` ve `transcribed` işaretlerini
   temizleyen hiçbir şey yok; bir insanın dökümü okuyup onayladığını
   söyleyebileceği bir komut yok. ADR-0016 bunu ayrı bir karar olarak
   bıraktı.

**Ses dökümü tamam** (ADR-0016): `core/transcribe` paketi ve
`evomem transcribe` komutu yazıldı. OpenAI-uyumlu
`POST /v1/audio/transcriptions` arayüzünü hedefliyor;
`EVOMEM_TRANSCRIPTION_URL` yoksa özellik kapalı ve komut bunu söyleyip çıkar.
Telegram'dan `getFile` + indirme, 20 MB sınırı, hatalar nota yazılıyor,
MCP iki işareti de modele gösteriyor. Mobildeki kayıtlar kapsam dışı.

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
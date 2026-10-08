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
| 3 Flutter mobil uygulama | kısmen — iskelet + sqflite + sync + settings + ses kaydı + testler; **Android/iOS derleme dosyaları yok** |
| 4 Telegram + Jira + HTTP girişi | tamam |
| 5 Sync + arşivleme | tamam |

`plan.md`'deki kutular işaretli; sapmalar o dosyada not düşülmüş ve
gerekçeleri `adr/`'de.

## Ne var

Tek statik Go ikilisi, çalışma zamanı bağımlılığı yok. 11 tane paket:

```
shared/models      Note, açık uçlu source_type, metadata uzatma noktası,
                   elde yazılmış ULID, tainted işareti
shared/audio       nota ait kayıtların dosya deposu, silme dahil
shared/database    tek yazıcılı iki havuz, FTS5 arama, delta izleme,
                   tombstone, öneriler, arşivleme
core/mcp           stdio JSON-RPC, spec'e göre yazılmış, iki protokol dönemi
core/api           POST /ingest, /ingest/audio + Telegram ve Jira webhook'ları
core/sync          Remote arayüzü, worker, PostgreSQL transportu
cmd/evomem         14 alt komut (mcp, serve, pull, restore, transcribe dahil)
apps/mobile        Flutter 3.47 (Riverpod 3, go_router, sqflite, http, very_good_analysis, l10n)
                   mobil şema v4: notes.remote_id
                   record 7.1.1 + path_provider 2.1.6 (ses kaydı)
```

Şema sürümü **3**. Kayıtlar şemada değil, veritabanının yanındaki `audio/`
dizininde. Bağımlılıklar: `modernc.org/sqlite`, `jackc/pgx/v5`,
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
- **Ses yükleme → döküm → silme**: derlenmiş ikiliyle uçtan uca; yükleme 204,
  dosya `audio/<id>.ogg` olarak 0600 izniyle yazıldı, döküm içeriği
  değiştirdi, `evomem delete` dosyayı da götürdü, dizin boş kaldı. Hatalı
  istekler: bozuk id / yol denemesi / olmayan not / eksik parametre → 400,
  yanlış token → 401
- **Ses dökümü (Telegram yolu)**: sahte bir OpenAI-uyumlu servis ve sahte bir Bot API ile
  uçtan uca — derlenmiş ikiliyle, gerçek HTTP; `getFile` bir kez gerçek
  Telegram'a da gitti ve `Unauthorized` döndü (hata yolu ve token gizleme
  doğrulandı)
- **Flutter mobil**: sqflite persistence, sync, settings, ses kaydı — 67 test,
  `flutter analyze` temiz, web release build'i geçiyor
- **`remote_id`**: yerel bir HTTP sunucusuna karşı; dönen id saklanıyor,
  `updated_at` oynamıyor, ikinci push hiç istek atmıyor, yarıda kalan batch
  tekrarlandığında yalnızca eksik notu gönderiyor. v3→v4 migration gerçek bir
  v3 dosyası yaratılıp uygulamanın açılış yolundan geçirilerek denendi.

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

1. **Android/iOS derleme dosyaları yok.** `android/` ve `ios/` ağaçlarında ne
   `build.gradle`, ne `settings.gradle`, ne `Podfile`, ne `Runner.xcodeproj`
   var — uygulama bugüne kadar yalnızca web için derlenmiş. Ses kaydı yazıldı
   ama **gerçek bir mikrofonla hiç denenmedi ve bu dosyalar olmadan
   denenemez**. `record` 7.1.1'in istediği minSdk 23 ve iOS 12 de yazılacak
   bir yer bulamadı (manifest ve Info.plist'e yorum olarak düşüldü).
2. **Dökümü onaylama akışı.** `tainted` ve `transcribed` işaretlerini
   temizleyen hiçbir şey yok. ADR-0016 ayrı bir karar olarak bıraktı.
3. **Mobilde düzenlenen not sunucuya gitmiyor.** `/ingest` güncelleme
   yapamıyor ve `remote_id`'si olan not ikinci kez gönderilmiyor, yani uzak
   kopya ilk gönderildiği hali koruyor. Çift kayıt yerine bayat kayıt —
   bilinçli takas, ama bir güncelleme yolu gerekiyor.

**Ses kaydı tamam** (ADR-0018, telefon tarafı): `record` 7.1.1 ile mono/16 kHz
m4a kaydı, kaydı tarif eden ve `awaiting_transcription` işaretli not, ve not
kabul edildikten sonra dosyanın `POST /ingest/audio`'ya yüklenmesi. Zincir
uçtan uca gerçek sunucuya karşı denendi; **gerçek mikrofonla denenmedi**.

**`remote_id` tamam** (ADR-0018): telefon artık `/ingest`'ten dönen id'yi
`notes.remote_id`'ye yazıyor (mobil şema v4), ve `remote_id`'si olan notu
ikinci kez göndermiyor. İki şeyi birden kapattı: ses artık bir nota
bağlanabilir, ve yarıda kalan bir batch güvenle tekrarlanabilir.

**Ses yükleme tamam** (ADR-0018, sunucu tarafı): `POST /ingest/audio?note=<id>`
kayıtları veritabanının yanındaki `audio/` dizinine yazıyor,
`shared/audio` deposu üç yerin ortak noktası, `core/transcribe`'ın yerel
`Fetcher`'ı onları ağsız ve kimlik bilgisiz okuyor. **Not silinince dosya da
siliniyor** — arşivleme ve restore dahil. Telefon tarafı yazılmadı.

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
# Durum

Bu dosya **her oturum sonunda üzerine yazılır**: işler şu an nerede, sırada ne
var. Kronolojik kayıt `ilerleme.md`'de; burası anlık görüntü.

Son güncelleme: 2026-10-09 (on ikinci oturum) · Sürüm: **v0.1.1 yayında**
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
                   tombstone, öneriler, kümeler, arşivleme
core/mcp           stdio JSON-RPC, 10 araç (6 okur, 1 önerir, 3 gruplar)
core/api           POST /ingest, /ingest/audio, PUT + DELETE /notes/{id},
                   GET /notes + /clusters, /connections + /organize (panel),
                   CORS, Telegram ve Jira webhook'ları
core/organize      kümesiz notları OpenAI-uyumlu bir modele gruplatır
core/sync          Remote arayüzü, worker, PostgreSQL transportu
cmd/evomem         20 alt komut (mcp, serve, pull, restore, transcribe, endorse dahil)
apps/mobile        Flutter 3.47 (Riverpod 3, go_router, sqflite, http, very_good_analysis, l10n)
                   mobil şema v6: notes.remote_id/remote_updated_at, deletions.remote_id
                   record 7.1.1 + path_provider 2.1.6 (ses kaydı)
                   sqflite_common_ffi_web 1.2.0 (tarayıcı deposu)
```

Şema sürümü **5**. Kayıtlar şemada değil, veritabanının yanındaki `audio/`
dizininde. Bağımlılıklar: `modernc.org/sqlite`, `jackc/pgx/v5`,
ikisi de vendor'lı ve saf Go.

## Çalıştırma (sürekli kullanım)

```sh
cp .env.example .env     # üç sır doldurulur
docker compose up -d     # db + server + web, hepsi loopback'te
docker compose run --rm sync
```

Web <http://localhost:8080>, sunucu <http://localhost:8787>, PostgreSQL
`127.0.0.1:5432`. Ayrıntı `docs/docker.md`'de.

**Dikkat:** `compose.yaml`'da `server` servisinde `EVOMEM_POSTGRES_DSN` var
ama **sunucu bugün onu okumuyor** — yalnızca `evomem sync` okuyor. Sunucu
hâlâ volume'daki SQLite dosyasını tutuyor; PostgreSQL şimdilik ADR-0012'nin
aynası. ADR-0028 bunu değiştiriyor, göç henüz başlamadı.

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
evomem endorse <id>                   # dışarıdan gelen notu sahiplen
evomem clusters                       # ajan neyi nasıl gruplamış
evomem clusters -show <id>            # bir kümenin içi
evomem clusters -delete <id>          # gruplamayı geri al (notlar kalır)
evomem connect                        # bağlı kaynaklar ve son durumları
evomem connect -add jira …            # kaynak bağla (token stdin'den)
evomem pull-sources                   # bağlı kaynakların API'sinden çek
evomem organize [-dry-run]            # bağlı modele kümesiz notları gruplat
```

## Neyin doğrulandığı, neyin doğrulanmadığı

**Gerçekten çalıştığı görüldü:**

- SQLite deposu, FTS5 arama, eşzamanlı yazma — 384 Go testi
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
- **Flutter mobil**: sqflite persistence, sync, settings, ses kaydı, kümeler, proje seçimi — 101 test,
  `flutter analyze` temiz, web release build'i geçiyor
- **Kümeler uçtan uca, tarayıcıda**: ajan MCP ile iki notu grupladı → Go
  sunucusu `GET /clusters` ile verdi → Flutter web uygulaması listeledi ve
  detayında notları kaynaklarıyla (`jira`, `mobile`) gösterdi. Ayarlar
  arayüzden girildi, CORS gerçek tarayıcıda çalıştı.
- **Sunucu tarafı kümeleme uçtan uca, tarayıcıda**: model panelden bağlandı,
  anahtar alanı temizlendi, "Group notes" düğmesi ancak o zaman belirdi,
  basıldı → `POST /organize` 200 → sahte OpenAI-uyumlu sunucu doğru kabloyu
  gördü (Bearer, model adı, system+user, `json_object`,
  `max_completion_tokens`, `max_tokens` yok) → üç not tek kümeye girdi.
  Uydurulmuş id düştü; anahtar veritabanında düz metin değil.
- **Docker uçtan uca**: `db` + `server` + `web` ayakta, sunucu 12 ucu
  yayınladı, `/ingest` 201, `sync` notu PostgreSQL'e yazdı. Web konteyneri
  her dosyayı doğru içerik tipiyle servis etti; CORS izin verilen origin'e
  başlık veriyor, başkasına vermiyor.
- **Proje seçimi tarayıcıda**: konteynerden açılan uygulamada `default`'a
  not yazıldı, `isler` projesi açıldı, liste boşaldı, oraya not yazıldı ve
  **tam yenilemeden sonra `isler`'de geri geldi** — seçim hatırlanıyor.
- **Panel uçtan uca, tarayıcıda**: Jira panelden bağlandı (201), liste onu
  gösterdi, "Sync now" `POST /connections/pull` attı (200), sahte Jira iki
  sayfa gördü (ilk istekte `nextPageToken` yok, ikincide `page-2`), iki issue
  `origin: jira` + `tainted` not oldu. API token alanı gönderimden sonra
  temizlendi; token hiçbir okuma ucundan geri dönmüyor.
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

## Yön (2026-10-09, kullanıcıdan)

**Web önce, mobil sonra.** Not döngüsünün tamamı — not oluşturma, **notların
yapay zekâ ile kümelere oturması ve organize edilmesi** — önce tarayıcıda
denenebilsin; mobil ondan sonra geliştirilsin. Web bir yan ürün değil, test
yüzeyi.

**Mimari genişletilebilir olsun:** sırada notları organize eden yapay zekâ ve
onu Claude'a bağlayan **MCP özellikleri** var. Bu yönde istekler gelecek,
sökülmeden yerine konamayacak hiçbir şey tasarlanmamalı.

Önceliklendirme sonucu: yalnızca telefonda karşılığı olan iş (platform derleme
dosyaları, gerçek mikrofon) tarayıcı yolunu gerçek kılan işin altında.

## Sırada — kod

Öncelik sırasına göre, her biri tek oturumluk iş:

0. **Anthropic'in kendi Messages API'si konuşulamıyor.** `core/organize`
   OpenAI-uyumlu `chat completions`'ı hedefliyor; Claude'u bu uçtan
   konuşturmak için önüne bir uyum katmanı gerekir. `Grouper` arayüzü buna
   kapalı değil (ADR-0027).

1. **ADR-0028'in göçü başlamadı.** Sunucunun deposu PostgreSQL olacak;
   Docker, volume ve kablolama hazır ama `shared/database` hâlâ yalnızca
   SQLite konuşuyor. Beş aşamaya bölündü, ADR'de sırası yazılı. Bu, kalan
   işlerin en büyüğü.

2. **Tarayıcıda ses kaydı çalışmıyor.** `record`'un web desteği var ama kaydı
   yazdığımız yolu `path_provider` veriyor ve onun web uygulaması yok.
   Tarayıcının tam bir test yüzeyi olmasının önündeki sıradaki engel.

1. **Android/iOS derleme dosyaları yok.** `android/` ve `ios/` ağaçlarında ne
   `build.gradle`, ne `settings.gradle`, ne `Podfile`, ne `Runner.xcodeproj`
   var — uygulama bugüne kadar yalnızca web için derlenmiş. Ses kaydı yazıldı
   ama **gerçek bir mikrofonla hiç denenmedi ve bu dosyalar olmadan
   denenemez**. `record` 7.1.1'in istediği minSdk 23 ve iOS 12 de yazılacak
   bir yer bulamadı (manifest ve Info.plist'e yorum olarak düşüldü).
4. **Toplu onay yok.** `evomem endorse` tek notu alıyor; proje ya da kaynak
   bazında onay ADR-0021'de bilinçli olarak dışarıda bırakıldı (sürtünme
   koruma sayıldı). Bunun tiyatro mu koruma mı olduğu kullanımla anlaşılacak.
5. **Sunucuda silinen not telefonda kalıyor.** `evomem pull` notları okuyor,
   mezar taşlarını okumuyor. Diğer yön; pull tarafının `deletions` tablosunu
   da okuması gerekiyor. ADR-0020 kapsam dışı bıraktı.

**Kümeler tarayıcıda görünüyor**: `lib/src/clusters/` (model, HTTP servisi,
sağlayıcılar) ve iki ekran — liste ve detay. Kümeler yerelde saklanmıyor,
her açılışta sunucuya soruluyor (ADR-0024). Yapılandırılmamış / ulaşılamayan /
okunamayan durumları ayrı ayrı açıklanıyor, çünkü boş ekranın üç ayrı anlamı
var. Detayda her notun **kaynağı** ve işaretleri (dışarıdan, makine dökümü,
onaylı) gösteriliyor.

**Proje seçilebiliyor**: uygulama çubuğundaki seçici hangi projeye
bakıldığını söylüyor ve değiştiriyor; not listesi, kümeler, yazılan not,
kaydedilen ses ve panelin gruplatma düğmesi hepsi ona göre. Projeler
**depodan** soruluyor (`NotesStore.projects()`), çünkü bir proje altında bir
not durduğu sürece vardır; seçili olan, henüz notu olmasa da listede
tutuluyor. Seçim hatırlanıyor (`ProjectMemory` portu). Yol boyunca iki hata
çıktı: proje değişince `build()` yeniden koştuğu için `late final` alanlar
patlıyordu, ve `add()` notu projesine bakmadan listeye koyuyordu.

**Sunucu kendi gruplayabiliyor** (ADR-0027): panele yazılan bir model
anahtarıyla `evomem organize` ve panelin "Group notes" düğmesi kümesiz
notları bir modele gruplatıyor. ADR-0023 bu seçeneği reddetmişti; ADR-0027
onu değiştiriyor ve neyin ayakta kaldığını yazıyor — **zamanlayıcı yok**,
düğmeye basan biri var. Arayüz OpenAI-uyumlu `chat completions`, şekli
OpenAI'ın kendi OpenAPI belgesinden doğrulanmış. Anahtar aynı keyring'de
(`connections`, `source_type = "model"`, aynı mühür); `pull-sources` o satırı
adıyla atlıyor. Yalnızca **kümesiz** notlar veriliyor, yani koşu grup ekler
ve hiçbirini dağıtmaz; gönderilmemiş bir id düşürülüyor. Tavan 200 not.
**Bedeli açıkça yazılı**: anahtar varken düğmeye basmak, gruplanacak notların
metnini anahtarın gösterdiği sunucuya gönderir — panel bunu anahtarın
girildiği yerde söylüyor.

**Panel tamam** (ADR-0026): bir kaynak artık tarayıcıdan bağlanıyor ve sync
tarayıcıdan tetikleniyor. Dört uç — `GET/POST /connections`,
`DELETE /connections/{id}`, `POST /connections/pull` — hepsi aynı token'ın
arkasında. Uygulamada `/sources` ekranı: kaynak listesi, Jira formu,
"Sync now". **Mühürlü sır tarayıcıya hiç dönmüyor**; okuma uçları yalnızca
adı, adresi ve son durumu veriyor.

**Çekme bağlayıcıları tamam** (ADR-0025): `evomem connect` bir kaynağı
bağlıyor, `evomem pull-sources` onların API'sini çağırıyor. İlk bağlayıcı
**Jira** — Atlassian'ın kendi referansından doğrulanmış
`GET /rest/api/3/search/jql`, e-posta:token Basic auth, `nextPageToken`
sayfalaması. Token **mühürlü** saklanıyor (AES-256-GCM, anahtar
`EVOMEM_SECRET_KEY`'den): çalınmış bir `evomem.db` tek başına yetmiyor.
Çekilen her not **tainted** — kimsenin okumadığı üçüncü taraf metni.
Şema **5**: `connections`.

**Okuma yolu açıldı** (ADR-0024): `GET /notes`, `GET /clusters`,
`GET /clusters/{id}` — hepsi aynı token'ın arkasında, token yoksa endpoint de
yok. **Otorite türe göre ayrıldı**: not yazıldığı cihaza ait ve tek yönlü
itilir (ADR-0012 korunuyor), küme yalnızca sunucuda yaşar ve istemciler onu
HTTP ile okur. İki yönlü sync açılmadan telefonun kümeleri görmesi böyle
mümkün oluyor; bedeli kümelerin çevrimdışı görünmemesi.

**`EVOMEM_CORS_ORIGIN`** olmadan tarayıcı bu uçları çağıramaz. Boş bırakmak
hiçbir origin'e izin vermiyor ve `*` kabul edilmiyor.

**Mobil artık bir kaynak** (ADR-0024): telefon notları `manual` yerine
`mobile` olarak gönderiyor, yani `GET /notes?source=mobile` çalışıyor.
Tarayıcıda yazılan not `manual` kalıyor — tarayıcı bir kaynak değil, hafızanın
üzerinde çalışılan yer.

**Kümeleme tamam** (ADR-0023, Go tarafı): ajan MCP ile notları gruplayabiliyor.
Beş yeni araç (`create_cluster`, `update_cluster`, `delete_cluster`,
`list_clusters`, `get_cluster`), iki yeni tablo (`clusters`, `cluster_notes`,
şema **4**), ve `evomem clusters` — ajanın yaptığını insanın ajan olmadan
görebilmesi için. Gruplama **öneri kuyruğundan geçmiyor**: ADR-0013'ten
bilinçli ilk sapma, gerekçesi ADR-0023'te. Hiçbir araç bir notun ne
söylediğini değiştirmiyor.

**Web artık gerçekten çalışıyor** (ADR-0022): depo `NotesStore` portunun
arkasında, `DatabaseHelper` `kIsWeb` olduğunda `sqflite_common_ffi_web`'e
geçiyor — şema ve bütün migration'lar aynı kalıyor. Tarayıcıda yazılan not
yenilemeden sağ çıkıyor, elle doğrulandı. **`databaseFactoryFfiWebNoWebWorker`
kullanılıyor**: paketin varsayılan shared-worker fabrikası burada açılışta
null döndü ve wasm'ı hiç indirmedi. Maliyeti ADR'de: sqlite UI isolate'inde
koşuyor, ve aynı origin'deki iki sekme kilitlemesiz bir sanal dosya sistemine
yazabiliyor.

**Açılışta not okuma düzeltildi** (ADR-0022): `loadNotes()` hiçbir yerden
başlangıçta çağrılmıyordu — **her platformda** her açılış dolu bir
veritabanının üstünde boş liste gösteriyordu. Hiçbir test yakalamamıştı,
çünkü her test notlarını kendi oturumunda ekleyip yine kendi oturumunda
doğruluyor.

**Onay tamam** (ADR-0021): `evomem endorse <id>` bir insanın notu okuyup
sahiplendiğini kaydediyor. `tainted` kalkıyor, `transcribed` kalıyor — ikisi
farklı iddia ve bir okuma yalnızca birini çözüyor. İddia silinmiyor:
`was_tainted` ve `endorsed_at` metadata'da, `origin` yerinde kalıyor. MCP
aracı değil (ADR-0008 salt-okunur), geri alınamıyor.

**Silme tamam** (ADR-0020): `DELETE /notes/{id}` satırı siliyor, aynı
işlemde mezar taşı yazıyor (PostgreSQL aynasına taşıyan şey o) ve notun ses
kaydını da götürüyor. Telefonda yerel bir mezar taşı kuyruğu var (mobil şema
v6: `deletions.remote_id`); kuyruk bir kayıt değil, sunucu aldıktan sonra
satır siliniyor. 404 "zaten gitmiş" sayılıyor.

**Düzenleme tamam** (ADR-0019): `PUT /notes/{id}` bir notun ne söylediğini
değiştiriyor — kimliğini, projesini, kaynağını değil; `updated_at`'i sunucu
koyuyor. Telefon artık `isPushed` notu atlamıyor, değiştiyse PUT ediyor.
Mobil şema v5: `notes.remote_updated_at`, yarıda kalan bir batch tekrarlanınca
değişmemiş notu aynaya yeniden yazmamak için. Olmayan nota PUT **404** ve
yeniden yaratılmıyor.

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
# Durum

Bu dosya **her oturum sonunda üzerine yazılır**: işler şu an nerede, sırada ne
var. Kronolojik kayıt `ilerleme.md`'de; burası anlık görüntü.

Son güncelleme: 2026-10-07 · Son commit: `914c61b` · CI: yeşil

---

## Fazlar

| Faz | Durum |
| - | - |
| 1 Monorepo + SQLite deposu | tamam |
| 2 MCP sunucusu | tamam |
| 3 Flutter mobil uygulama | **bloke** — Flutter SDK kurulu değil |
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
cmd/evomem         13 alt komut
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

**Hiç denenmedi:**

- Gerçek bir Telegram botu veya gerçek bir Jira instance'ı
- Gerçek bir bulut PostgreSQL'i (`sslmode=require` ile uzak sunucu)
- Flutter tarafı (hiç yazılmadı)

---

## Sırada — [SEN]

Bunlar bende değil, sende:

1. **Flutter SDK kur** → Phase 3'ün kilidi.
   `brew install --cask flutter`, sonra `flutter doctor`.
2. **DCO app + branch koruması** GitHub repo ayarlarından. İmzalar atılıyor
   (`git commit -s`) ama kontrol eden bir şey yok.
3. **Tünel kur** (cloudflared veya ngrok) → Telegram ve Jira webhook'larının
   `evomem serve`'e ulaşması için. `evomem serve` TLS sunmuyor, bilinçli.
4. **Gerçek bot/webhook ile dene.** Telegram `setWebhook`, Jira'da gizli
   anahtarlı webhook. `docs/api.md` ikisinin de adımlarını yazıyor.
5. **Bulut PostgreSQL'i seç** ve `EVOMEM_POSTGRES_DSN` ile dene.

## Sırada — kod

Öncelik sırasına göre, her biri tek oturumluk iş:

1. **Phase 3: Flutter uygulaması** (Flutter kurulunca). `plan.md` Phase 3:
   `apps/mobile`, Go şemasıyla eşleşen yerel depo (`sqflite` veya `drift`),
   hızlı metin yakalama arayüzü, arka planda ses kaydı → yerel dosya +
   veritabanında referans. **Karar gerekecek:** mobil taraf doğrudan
   PostgreSQL aynasına mı bakacak, yoksa `evomem serve`'e mi yazacak?
2. **Telegram çoklu proje yönlendirmesi.** Şu an bir bot = bir proje
   (`EVOMEM_TELEGRAM_PROJECT`). Seçenekler: `chat_id` → proje eşlemesi,
   mesajdaki `#etiket`, bot komutu. Üçü çelişiyor, biri seçilmeli.
   `telegram_chat_id` metadata'da saklı olduğu için migration gerekmiyor.
3. **Pull / restore.** Senkronizasyon tek yönlü; ikinci bir makine aynadan
   okuyamıyor, aynadan geri yükleme yok. Ayna şeması ikisini de mümkün
   kılacak kadar sade bırakıldı (ADR-0012'nin son maddesi). Kendi ADR'sini
   gerektirir.
4. **Servis tanımı.** `evomem sync` ve `evomem serve` elle çalıştırılıyor.
   macOS için launchd plist, Linux için systemd unit.
5. **Ses dökümü.** Telegram ses mesajları `awaiting_transcription: true` ve
   `telegram_file_id` ile duruyor; `getFile` ile indirip döküme çevirecek bir
   şey yok. Bir döküm bağımlılığı gerektirir → ADR.
6. **Sürüm ve dağıtım.** forgelore'da `release.sh` + `npm-pack.sh` +
   attestation'lı release workflow var; burada hiç yok. İlk sürümü kesmek
   istediğinde o kalıp alınabilir.

## Bilinen sınırlar (hata değil, karar)

- **Türkçe aramada `ı` → `i` katlanmıyor.** `veritabani` yazınca
  `veritabanı` bulunmuyor. ğ/ş/ç/ö/ü katlanıyor. Gövdeleme hiç yok.
  Düzeltmesi Türkçe-farkında tokenizer = yeni bağımlılık. → ADR-0003
- **Jira imzasında replay penceresi yok.** Yakalanmış bir teslim, gizli
  anahtar değişene kadar tekrar oynatılabilir. → ADR-0010
- **Yeni bir adaptör `MarkTainted` çağırmayı unutursa** özellik sessizce
  kaybolur. Model paketinden zorlanamıyor; gözden geçirme maddesi. → ADR-0009
- **Arşivleme senkronize edilmemiş notu silmiyor**, yani sync kurulmadan
  `evomem archive` hiçbir şey yapmıyor (ve nedenini söylüyor). → ADR-0012

## Yeni oturuma nasıl başlanır

1. Bu dosyayı oku.
2. `CLAUDE.md` — mimari kısıtlar ve çalışma kuralları.
3. `ilerleme.md`'nin **son bölümü** — en son ne yapıldı.
4. İlgili `adr/` kaydı — bir karara dokunacaksan önce onu oku; kararlar
   yeniden tartışılmaz, gerekçesiyle değiştirilir.

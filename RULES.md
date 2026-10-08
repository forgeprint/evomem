# Evomem Çalışma Kuralı (Workflow Rules)

Bu dosya, Evomem projesinde her oturumda izlenecek zorunlu kuralları tanımlar. Her yeni sohbet/oturum başında bu dosya okunur ve kurallara uyulur.

---

## 1. İş Başlangıcı: Fazlandırma (Phase Planning)

Her yeni görev/özellik başladığında **mutlaka** şu adımlar yapılır:

### 1.1 Plan Yaz
- `docs/plan.md` dosyasına yeni faz eklenir
- Hangi dosyaların değişeceği, hangi testlerin yazılacağı, kabul kriterleri netleştirilir
- Bağımlılıklar (diğer paketler, dış servisler) belirtilir

### 1.2 Issue/Task Oluştur
- GitHub Issues'da ilgili task açılır (varsa)
- Branch isimlendirme: `feat/<kısa-ad>`, `fix/<kısa-ad>`, `chore/<kısa-ad>`

### 1.3 Branch Kuralı
```bash
git checkout -b feat/<kısa-ad>  # yeni özellik
git checkout -b fix/<kısa-ad>   # hata düzeltmesi
git checkout -b chore/<kısa-ad> # bakım/refactor
```

---

## 2. Geliştirme Süreci

### 2.1 Kod Yazım Standartları
- **Go**: `gofmt`, `go vet`, `go test` geçmeli
- **Flutter**: `dart format`, `flutter analyze`, `flutter test` geçmeli
- Test yazmadan kod yazılmaz (TDD tercih edilir)
- ADR yazılmadan mimari karar verilmez

### 2.2 Commit Mesaj Formatı
```bash
git commit -s -m "<type>(<scope>): <konu>

<gerekçe>

<etkilenilen dosyalar>"
```

**Type'lar**: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `ci`

**Örnek**:
```bash
git commit -s -m "feat(sync): add pull/restore CLI commands

Add evomem pull and evomem restore commands with cursor-based
delta sync implementation per ADR-0015.

cmd/evomem/sync.go: add cmdPull, cmdRestore
core/sync/sync.go: add Pull/Restore methods"
```

### 2.3 DCO İmzası Zorunlu
```bash
git commit -s  # Developer Certificate of Origin imzası
```

---

## 3. İş Bitişi: Faz Kapatma (Phase Closing)

Her görev bitiminde **mutlaka** şu adımlar yapılır:

### 3.1 Test ve CI Doğrulama
```bash
# Go backend
./scripts/ci.sh          # fmt, vet, test, crosscheck, gitleaks

# Flutter mobile
cd apps/mobile
flutter analyze
flutter test
flutter build web --release
```

### 3.2 Dokümantasyon Güncelleme (Zorunlu)
| Dosya | Ne Güncellenir |
|-------|----------------|
| `docs/plan.md` | Faz kutusu ✅, sapmalar notlanır |
| `docs/durum.md` | **Her oturum sonunda** tamamen yeniden yazılır |
| `docs/ilerleme.md` | Kronolojik kayıt eklenir (ne yapıldı, kararlar, açıklar) |
| `docs/adr/NNNN-*.md` | Mimar karar varsa yeni ADR eklenir |
| `CLAUDE.md` | Gerekirse kural güncellenir |

### 3.3 ADR Kuralı
- Her mimari karar için `docs/adr/NNNN-<konu>.md` yazılır
- Format: Context, Decision, Consequences
- Numara sıralı artar (0014, 0015, ...)

### 3.4 Push ve PR
```bash
git push origin <branch>
# GitHub'da PR açılır
# CI yeşil olmalı
# En az 1 approval (kendiniz veya bot)
# Merge sonrası branch silinir
```

---

## 4. Oturumlar Arası Devamlılık (Continuity)

### 4.1 Bir Sonraki Oturum İçin Hazırlık
Her oturum sonunda şu dosyalar **kesinlikle** güncellenir:

| Dosya | İçerik |
|-------|--------|
| `docs/durum.md` | **Anlık görüntü**: Faz durumu, ne var, komutlar, ne doğrulandı, sıradaki işler (SEN vs kod), bilinen sınırlar, yeni oturum okuma sırası |
| `docs/ilerleme.md` | Kronolojik defter: ne yapıldı, kararlar, açıklar (eklenir, silinmez) |
| `docs/plan.md` | Kutular işaretlenir, sapmalar not edilir |

### 4.2 Yeni Oturum Başlangıç Sırası
Yeni sohbet başladığında **sırayla** okunur:
1. `docs/durum.md` — anlık görüntü
2. `CLAUDE.md` — mimari kısıtlar, çalışma kuralları
3. `docs/plan.md` — faz planı, sapmalar
4. `docs/ilerleme.md` **son bölümü** — en son ne yapıldı
5. İlgili `docs/adr/` — karar verilecek konu varsa

---

## 5. Git & Merge Kuralları

### 5.1 Branch Koruması
- `main` branch: **korumalı** (Settings → Branches)
- Push yapılmaz, PR zorunlu
- CI yeşil olmadan merge edilmez

### 5.2 Merge Stratejisi
- **Squash and merge** (varsayılan)
- Commit mesajı PR başlığına editlenebilir
- Merge sonrası branch silinir: `git push origin --delete <branch>`

### 5.3 Tag & Release
```bash
# Release hazır
./scripts/release.sh v0.x.y  # 5 target build + SHA256SUMS
git tag -a v0.x.y -m "v0.x.y"
git push origin v0.x.y
# GitHub Release draft → publish
```

### 5.4 DCO & İmza
- Tüm commit'ler `-s` ile imzalanır
- GitHub'da DCO app etkin olmalı

---

## 6. CI/CD Pipeline

### 6.1 GitHub Actions (`.github/workflows/ci.yml`)
| Job | Ne Yapar |
|-----|----------|
| `go` | `./scripts/ci.sh` (fmt, vet, test, crosscheck, gitleaks) |
| `flutter` | `flutter pub get`, `gen-l10n`, `format`, `analyze`, `test`, `build web` |
| `crosscheck` | `./scripts/crosscheck.sh` (5 target) |

### 6.2 Yerel CI Çalıştırma
```bash
./scripts/ci.sh  # Go backend
cd apps/mobile && flutter analyze && flutter test && flutter build web --release
```

---

## 7. Sizin Yapmanız Gerekenler (Human Tasks)

Bu işler **kod ile yapılamaz**, siz GitHub UI / lokal makinede yaparsınız:

| Görev | Nerede |
|-------|--------|
| Branch protection (`main`) | GitHub Settings → Branches |
| DCO App enable | Settings → Apps → "DCO" |
| Tunnel (cloudflared/ngrok) | Lokal makine |
| Telegram bot test | Tunnel + `setWebhook` |
| Jira webhook test | Jira UI → Webhook |
| Cloud PostgreSQL test | `EVOMEM_POSTGRES_DSN` |
| Telegram routing kararı | ADR-0014'onuzda karar verin |

---

## 8. Yasaklar (Don'ts)

| Yasak | Sebep |
|-------|-------|
| `main` branch'e doğrudan push | Branch protection bozulur |
| Test yazmadan kod yazma | Regresyon riski |
| ADR'siz mimari karar | Bilgi kaybı, tekerrür |
| `docs/durum.md` güncellemesiz bitirme | Sonraki oturum kaybolur |
| `git commit` without `-s` | DCO ihlali |
| CI kırmızıyken merge | Kırık main branch |
| `vendor/` manuel değiştirme | `go mod vendor` kullanılır |

---

## 9. Şablonlar (Templates)

### 9.1 ADR Şablonu (`docs/adr/NNNN-konu.md`)
```markdown
# ADR-NNNN: <Konu>

**Status:** proposed/accepted/rejected · **Date:** YYYY-MM-DD

## Context
<Bağlam, neden bu karar gerekiyor>

## Decision
<Karar ne, nasıl implement edilecek>

## Consequences
- <Olumlu etkiler>
- <Olumsuz riskler>
- <Maliyet/performans etkisi>
```

### 9.2 Plan Güncelleme (`docs/plan.md`)
```markdown
## Phase X: <Ad>
- [ ] Alt görev 1
- [ ] Alt görev 2
- [x] Tamamlanan
```

---

## 10. Onay ve Uygulama

Bu kural **hükmünde**. İlk oturumda okunur, her oturumda uygulanır.
Değişiklik için PR + review gerekir.

---

*Son güncelleme: 2026-10-08 · Evomem v0.1.0*
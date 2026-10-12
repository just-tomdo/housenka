# Housenka – plán implementace

## Kontext

Vykazování práce v Bílém motýlu (BM) nemá žádnou validaci kombinací: k libovolnému kódu projektu jde přiřadit libovolný odkaz na issue, libovolnou činnost i libovolný čas. Výsledkem jsou chyby ve výkazech.

Housenka je lokální TUI aplikace v Go. Výkazy se zadávají a ukládají v ní a pro každý záznam vygeneruje URL do BM, která předvyplní formulář. Klíčová validace funguje takto: z URL issue se zjistí repozitář, podle configu se nabídnou **jen povolené kódy projektů**, podle kódu projektu **jen povolené činnosti** a čas se kontroluje na čtvrthodiny. Pravidla jsou v jednom YAML configu, který má zůstat lidsky udržitelný.

Výchozí stav je prázdný projekt: jen `go.mod` (`github.com/just-tomdo/housenka`, `go 1.26.1`). Vývoj probíhá v sandboxu – viz [CLAUDE.md](CLAUDE.md).

## Vstupy od uživatele (zjištěné formáty)

**URL issue:** `https://gitlab.example.com/clients/acme-web/-/work_items/636`
- všechno před `/-/` je cesta projektu; poslední segment je repozitář (`acme-web`), zbytek je skupina (`clients`, případně vnořené skupiny, např. `interni/...`)
- typ: `work_items` nebo `issues` (případně `merge_requests`), za ním číslo
- může mít kotvu `#note_123456`, ta se zachová v odkazu do BM, ale na rozpoznávání nemá vliv
- v seznamu výkazů se zobrazuje **skupina / repo / #číslo**, aby šlo správnost zkontrolovat očima

**URL do BM** (base + query parametry, kódování `+` pro mezery = `url.Values.Encode()`):
```
https://bm.example.com/vykazy/editor
  ?project=SHOPDEV
  &issue=<celá URL issue vč. #note>
  &hours=5,00          ← čárka, 2 desetinná místa
  &activity=DEV
  &date=9. 10. 2026    ← "D. M. YYYY", bez úvodních nul
  &description=<popis>
```

## Umístění souborů

Vše je v jednom adresáři **`~/.housenka/`** na všech platformách (Linux, macOS, WSL; na Windows `%USERPROFILE%\.housenka`):

```
~/.housenka/
  config.yaml
  entries/2026.json
```

Adresář jde přepsat proměnnou `HOUSENKA_HOME` (testy, druhý profil) a samotný config přepínačem `--config`.

## Config (YAML, `~/.housenka/config.yaml`)

```yaml
gitlab_hosts: [gitlab.example.com]          # jiné hosty = chyba validace

bm:
  base_url: https://bm.example.com/vykazy/editor
  open_command: [wslview]             # volitelné; jinak autodetekce podle OS

ui:
  theme: tokyonight                   # tokyonight | catppuccin | gruvbox
  nerd_font: true                     # ikony; false = Unicode fallback
  caterpillar: true                   # progress bar jako housenka 🐛

time:
  daily_goal_hours: 8                 # cíl dne pro progress bar
  max_entry_hours: 12                 # horní mez jednoho záznamu
  daily_warning_hours: 8              # jen varování v přehledu dne

activities:                           # globální číselník činností
  DEV:  { name: Vývoj webu – dynamický, productive: true }
  ANALYZA: { name: Analýza,                productive: true }
  REZIE:   { name: Režie / porady,         productive: false }
  # productive je povinné – žádný tichý default, ať se nic nezařadí špatně

projects:                             # číselník kódů projektů BM
  BETADEV:
    name: Beta – rozvoj
    activities: [DEV, ANALYZA]     # volitelné omezení; chybí = všechny
    default_activity: DEV          # volitelné
  BETASUP:
    name: Beta – podpora
  INTERNI:
    name: Interní režie
    issue: optional                   # required (default) | optional

repos:                                # GitLab cesta → povolené kódy projektů
  clients/beta-app:          [BETADEV, BETASUP]
  clients/acme-web:    [ACME]
  clients/shop-web: [SHOPDEV, SHOPSUP]
  interni/*:             [INT01]        # "/*" = celá skupina vč. podskupin; přesná cesta má přednost
```

Pravidla validace configu při startu (chyby se vypíšou všechny najednou, s konkrétním klíčem):
- striktní dekódování YAML (`KnownFields`), takže překlep v klíči je chyba, ne tiché ignorování
- každý kód v `repos` existuje v `projects`; každá činnost v `projects.*.activities` a `default_activity` existuje v `activities`; `default_activity` musí patřit do povolených činností
- `repos` klíče jsou buď přesná cesta, nebo `<prefix>/*`; žádné duplicity
- `base_url` je validní URL, `gitlab_hosts` není prázdné

Rozlišení repo → kódy: nejdřív přesná shoda cesty, pak nejdelší odpovídající `prefix/*`. Když se nic nenajde, je to **chyba** („repo X nemá v configu přiřazené kódy“) a uživatel dostane tip, kam ho do configu doplnit.

## Datový model a úložiště

```go
type Entry struct {
    ID          string    // krátké náhodné ID
    Date        civil date (time.Time, jen datum)
    IssueURL    string    // prázdné jen pro projekty s issue: optional
    ProjectCode string
    Activity    string
    Quarters    int       // čas v čtvrthodinách (2.75 h = 11) – žádná float aritmetika
    Description string
    Sent        bool      // uživatel potvrdil, že záznam odeslal do BM
    CreatedAt, UpdatedAt time.Time
}
```
**Úložiště: JSON rozdělený po rocích**, `~/.housenka/entries/2026.json`. Soubor patří k roku podle `Entry.Date`; když se při úpravě změní datum do jiného roku, záznam se přesune do jiného souboru. Zápis je atomický (temp soubor + rename) a týká se jen dotčeného roku. Soubory jsou čitelné, dají se zálohovat a verzovat v gitu.

Výkon: rok je zhruba 2 000–3 000 záznamů, tedy asi 0,5–0,7 MB JSON. Načtení i celý přepis souboru při uložení trvají jednotky milisekund, takže s tím problém není. TUI drží načtené roky v paměti. Týden přes přelom roku (např. 30. 12.–5. 1.) načte oba soubory. Rozhraní `Store` (`LoadYear`, `SaveYear`, nad tím `Range(from, to)`) umožní pozdější výměnu za SQLite bez zásahu do zbytku aplikace.

Při **načtení** se každý záznam znovu zvaliduje proti aktuálnímu configu. Neplatné záznamy (např. po změně configu) se v seznamu označí, ale nezahodí se.

## Validace záznamu (jádro, čistě testovatelné, bez TUI)

1. **Issue URL**: host musí být v `gitlab_hosts`, cesta musí obsahovat `/-/(issues|work_items|merge_requests)/<číslo>`; z URL se vytáhne skupina, repo a číslo.
2. **Projekt**: kód musí patřit do množiny povolené pro repo z URL. Bez issue jsou povolené jen projekty s `issue: optional`.
3. **Činnost**: musí být v povolených činnostech projektu.
4. **Čas**: přijímá `2.75`, `2,75`, `2:45`; musí být > 0, násobek 0,25 h a ≤ `max_entry_hours`.
5. **Datum**: validní, ne v budoucnosti (budoucí datum je jen varování).
6. **Popis**: neprázdný po oříznutí mezer.

## Struktura projektu

```
cmd/housenka/main.go          # vstup, subpříkazy, načtení configu
internal/config/              # typy, Load (strict YAML), Validate, výchozí cesty (XDG)
internal/gitlaburl/           # Parse(url) → {Host, Group, Repo, Kind, IID, Raw}
internal/rules/               # ProjectsForRepo, ActivitiesForProject, ValidateEntry
internal/hours/               # Parse("2,75") → quarters; Format(quarters) → "2,75" / "2.75"
internal/entry/               # Entry model
internal/store/               # JSON store (Load/Save/atomic write)
internal/bm/                  # BuildURL(cfg, entry) → string (formáty data/hodin)
internal/opener/              # otevření URL v prohlížeči (WSL-aware) + kopírování do schránky
internal/period/              # Day/Week(ISO)/Month: rozsah, Next/Prev, popisek
internal/stats/               # agregace za období: celkem, produktivní/neproduktivní (v čtvrthodinách),
                              # týdenní souhrn projekt → issue + textový/Markdown výstup
internal/tui/                 # app.go (model, klávesy, akce), view.go (seznam, detail, statusline),
                              # header.go (statistiky + housenka), form.go, status.go (týdenní souhrn),
                              # keymap.go, styles.go, theme/ (palety, ikony, barvy štítků)
config.example.yaml           # vzorový config s komentáři
```

Každý balíček v `internal/` kromě `tui` má table-driven testy.

## TUI (Bubble Tea v2 + Bubbles v2 + Lip Gloss v2)

Použité verze: `charm.land/bubbletea/v2` v2.0.10, `bubbles/v2` v2.2.1, `lipgloss/v2` v2.0.6 (API ověřené ze zdrojáků). Formulář je vlastní nad `textinput` (ne Huh) – kvůli živému přefiltrování selectů a vim klávesám.

Aplikace běží **přes celý terminál** (alt screen) a reaguje na změnu velikosti okna. Layout má tři pevné části:

```
┌ 🐛 Housenka ─ Den: po 12. 10. 2026 ────────── [Den] · Týden · Měsíc ┐
│ Celkem 7,50 h / 8 h   Produktivní 6,00 h · Neproduktivní 1,50 h       │
│ ████████████████████████████████████████░░░░░░░░░░  80 % / 20 %        │  ← STATISTIKY
├───────────────────────────────────────────────────────────────────────┤
│ Skupina  Repo           #     Kód       Činnost  Hod   Popis       St │
│ clients      acme-web    636   ACME    DEV   2,75  úprava …    ✓  │  ← SEZNAM
│ clients      beta-app          12    BETADEV   ANALYZA  1,00  analýza …   •  │
├───────────────────────────────────────────────────────────────────────┤
│ j/k pohyb · h/l den · a přidat · e upravit · o otevřít v BM · ? více  │  ← NÁPOVĚDA
└───────────────────────────────────────────────────────────────────────┘
```

**Horní panel se statistikami**
- součet hodin za zobrazené období; v denním režimu varování (barva) při překročení `daily_warning_hours`
- **barevný progress bar** produktivní (zelená) vs. neproduktivní (oranžová) s procenty a hodinami; počítá se z `activities.*.productive`
- tři období: **den / týden / měsíc**. Přímý výběr klávesami `1` / `2` / `3`, cyklické přepínání přes `Tab`. `h`/`l` posouvá o jedno zvolené období.
  - **den**: záznamy dne a bar za den
  - **týden** (po–ne, ISO týden): panel ukazuje poměr za týden, hodiny a průměr na den, k tomu řádek „sloupečků“ po–pá s mini bary. Seznam je seskupený po dnech se součtem u každého dne.
  - **měsíc**: totéž za měsíc, navíc počet odpracovaných dní a průměr na den
- **Týdenní souhrn pro teamový status** (klávesa `S` v týdenním režimu): modal se záznamy agregovanými podle **projektu → issue** (hodiny + spojené popisy). Tlačítko zkopíruje souhrn jako prostý text / Markdown do schránky, aby šel rovnou vložit do statusu. Formát generuje balíček `stats`, takže je testovatelný bez TUI.

**Seznam výkazů**
- sloupce: (Datum v týdenním/měsíčním režimu) · Skupina · Repo · #Issue · Kód · Činnost · Hodiny · Popis (zkrácený, celý v detailu) · Stav (✓ odesláno / • čeká / ⚠ neplatné)
- neplatné záznamy (např. po změně configu) jsou zvýrazněné a důvod je vidět v detailu

**Ovládání: vim-friendly, ale použitelné i bez znalosti vimu.** Každá akce má vim klávesu i „běžnou“ alternativu (`key.Binding` z Bubbles s více klávesami):

| Akce | Vim | Alternativa |
|---|---|---|
| pohyb v seznamu | `j` / `k`, `gg` / `G`, `Ctrl-d` / `Ctrl-u` | šipky, Home/End, PgDn/PgUp |
| předchozí / další období | `h` / `l` | ← / → |
| dnešek | `t` | — |
| den / týden / měsíc | `1` / `2` / `3` | Tab (cyklicky) |
| týdenní souhrn pro status | `S` | — |
| hledat ve vypsaných | `/`, `n` / `N` | — |
| přidat / upravit / duplikovat | `a` (`o`) / `e` (`Enter`) / `yy`→`p` (duplikát na zobrazený den) | Insert / Enter / `d` |
| smazat (s potvrzením) | `dd` | Delete |
| otevřít v BM / zkopírovat URL | `b` / `Y` | — |
| přepnout „odesláno“ | `s` | Space |
| nápověda / konec | `?` / `q`, `:q` | F1 / Ctrl-c |

Přesné obsazení kláves se doladí při implementaci tak, aby nekolidovalo. Je centralizované v jednom `keymap.go`, aby šlo později udělat přepsatelné v configu.

**Vzhled: barevný a hravý, ve stylu Neovimu s pluginy nebo Tuxedo**
- ucelená barevná paleta jako **téma** (vestavěná např. `tokyonight`, `catppuccin`, `gruvbox`), volba v configu `ui.theme`. Všechny barvy jsou definované na jednom místě (`styles.go`) přes Lip Gloss adaptivní barvy.
- statusline ve stylu lualine: nalevo barevný **„mode“ blok** (`NORMAL` / `INSERT` ve formuláři / `SEARCH`), uprostřed období, napravo součet hodin a počet neodeslaných záznamů
- zaoblené rámečky panelů; aktivní panel má výraznější barvu rámečku
- **barevné štítky (badges)** pro kód projektu a činnost. Barva projektu se odvodí deterministicky hashem kódu z palety, takže stejný projekt má vždy stejnou barvu a dá se poznat na první pohled. Produktivní činnosti mají zelený tón, neproduktivní oranžový.
- ikony stavu: Nerd Font ikony, nebo Unicode fallback (`ui.nerd_font: true|false`)
- **zábavné prvky**: progress bar jako housenka 🐛, která „roste“ s odpracovanými hodinami do denního cíle (volitelné `ui.caterpillar: true`, jinak klasický bar). Po splnění denního cíle krátká gratulace ve statusline. Toasty („Uloženo ✓“, „URL zkopírována“) mizí po 2 s.
- prázdný stav dne: přátelská hláška s nápovědou „stiskni `a` a vykaž první práci“

**Nápověda dole**: krátký řádek s nejdůležitějšími klávesami podle aktuálního kontextu (seznam × formulář); `?` rozbalí plnou nápovědu (komponenta `help` z Bubbles).

**Formulář přidání/úpravy** (pořadí řídí validaci):
1. Datum (výchozí dnešek)
2. Issue URL → hned po vyplnění se parsuje, pod polem se ukáže `clients / acme-web / #636` nebo chyba
3. Kód projektu: **select jen z povolených kódů pro repo**; jediná možnost se předvybere
4. Činnost: select jen z povolených činností projektu, předvybraná `default_activity`
5. Hodiny (validace čtvrthodin)
6. Popis

Formulář se otevírá jako modální okno přes seznam. Pohyb mezi poli je přes `Tab`/`Shift-Tab` (u selectů i `j`/`k`), `Esc` ruší, `Ctrl-s` ukládá. Po uložení se nabídne „Otevřít v BM?“, což urychlí obvyklý postup zadat → odeslat.

**Architektura TUI s ohledem na budoucí task manager:** kořenový model je „app“ se sdílenou hlavičkou a nápovědou a s přepínatelnými **views**. Teď existuje jen `worklog` view. Později přibude `tasks` view (kanban/TODO) jako další view a záznam výkazu dostane volitelné `TaskID`. Všechno ostatní se jen přidává, žádný přepis.

## Subpříkazy (bez TUI, pro skriptování/ladění)

- `housenka` spustí TUI
- `housenka config check` zvaliduje config a vypíše chyby; exit code ≠ 0 při chybě
- `housenka config init` zapíše `config.example.yaml` do výchozí cesty, pokud tam ještě nic není
- `housenka parse <issue-url>` vypíše skupinu/repo/číslo a povolené kódy projektů (rychlá kontrola mapování)

Parsování argumentů zajistí standardní `flag` + jednoduchý dispatch; cobra tu není potřeba.

## Pořadí implementace

1. **Základ**: `hours`, `gitlaburl`, `config` (Load + Validate), `rules` a testy. Tady je hlavní hodnota.
2. **`bm.BuildURL`** + test, který přesně reprodukuje uživatelův vzorový odkaz (SHOPDEV / DEV / 5,00 / 9. 10. 2026).
3. **`store`** (JSON po rocích, atomický zápis, přesun mezi roky) + `entry` + `period` + `stats` (vč. týdenního souhrnu) + testy.
4. **CLI**: `config check`, `config init`, `parse`.
5. **`opener`** s autodetekcí podle OS. Prohlížeč: macOS `open`, Windows `rundll32 url.dll,FileProtocolHandler`, WSL `wslview` → `cmd.exe /c start`, Linux `xdg-open`. Schránka: macOS `pbcopy`, Windows a WSL `clip.exe`, Linux `wl-copy` / `xclip`.
6. **TUI**: kostra (alt screen, téma, layout, statusline, keymap, nápověda) → seznam → statistiky a progress bar → formulář → akce → týdenní a měsíční režim → týdenní souhrn → zábavné prvky (housenka, toasty).
7. `config.example.yaml` + krátké README.

## Budoucí rozšíření (teď mimo rozsah, ale návrh je počítá)

- **GitLab API** (volitelný token v configu): ověření existence issue a předvyplnění popisu z titulku issue. Půjde o nový balíček `internal/gitlab` volaný ve formuláři po parsování URL; jádro se nemění.
- **Task manager** (kanban/TODO ve stylu Tuxedo, plněný ručně): nové `tasks` view, vlastní úložiště a provázání s výkazy přes `TaskID`; z úkolu se výkaz založí s předvyplněnou issue URL.
- Export dne nebo týdne (CSV), šablony častých záznamů, přepsatelný keymap v configu, SQLite store.

## Ověření

- `go test ./...`: table-driven testy pro parsování času (platné/neplatné vstupy, `2:45`, `0.3` → chyba), URL issue (work_items/issues, `#note`, vnořené skupiny, cizí host), rozlišení repo → kódy (přesná shoda vs. `/*`, nenalezeno), validaci configu (neexistující kód, překlep v klíči, chybějící `productive`), BM URL (shoda se vzorovým odkazem), období (ISO týden, týden přes přelom roku, Next/Prev), statistiky (poměr produktivní/neproduktivní za den, týden a měsíc; týdenní souhrn) a store (zápis a načtení roku, přesun záznamu při změně data přes rok).
- `go vet ./...` a `gofmt -l .` bez výstupu.
- Ručně: `housenka config init` → upravit config → `housenka config check` → `housenka parse https://gitlab.example.com/clients/acme-web/-/work_items/636` → v TUI přidat záznam, ověřit, že select nabízí jen povolené kódy a činnosti, a stisknout `b`, aby se otevřel BM s předvyplněným formulářem. Dál ověřit progress bar, přepínání den/týden/měsíc (`1`/`2`/`3`), týdenní souhrn (`S`) a jeho zkopírování, témata, vim i alternativní klávesy a chování při zmenšení terminálu.

## Stav implementace (2026-10-12)

Kroky 1–7 hotové. Odchylky od původního návrhu:
- TUI je zatím jeden balíček `internal/tui` s jedním modelem; oddělení „views“ pro budoucí task manager se udělá, až bude druhý view.
- Hledání `/` funguje jako filtr seznamu (místo skoku `n`/`N`).
- Duplikace: `c` = kopie na dnešek, `yy` + `p` = kopie na zobrazený den.
- Cíl týdne/měsíce = denní cíl × počet pracovních dní (po–pá); po splnění cíle se housenka změní v motýla 🦋.
- Témata jsou jen tmavá (tokyonight, catppuccin, gruvbox).

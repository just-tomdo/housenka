# Přehled issues z GitLabu

Volitelné rozšíření Housenky pro ty, kdo mají na sobě hodně issues a ztrácí v nich přehled. Issue přiřazená tobě se načítají z GitLabu, ty si je třídíš na vlastní nástěnku a rovnou z nich vykazuješ.

- Nová issue čekají v záložce **Nová** („2 nová issue“), dokud je nezařadíš nebo neskryješ.
- **Nástěnka** má sloupce podle tvých vlastních stavů (např. Na řadě, Rozpracováno, Čeká, Hotovo) s vlastními barvami. Issue mezi nimi přesouváš klávesami nebo myší a v rámci sloupce si je seřadíš.
- Každé issue má **poznámky**, datum přiřazení, termín z GitLabu, štítky a součet vykázaných hodin. Karta zežloutne, když na issue dlouho nesáhneš.
- **Vykázat** otevře formulář výkazu s vyplněným issue. Detail issue ukazuje všechnu práci na něm.
- Mrtvá issue **skryješ**; issue zavřené v GitLabu nebo přeřazené na někoho jiného zůstane se značkou, dokud ho neskryješ.

Rozšíření funguje v TUI i v prohlížeči se stejnými klávesami. Bez něj se Housenka chová jako dřív.

## Zapnutí

```sh
housenka issues init      # vytvoří ~/.housenka/issues.yaml – tím je rozšíření zapnuté
housenka issues token     # uloží token GitLabu (zadává se skrytě a hned se ověří)
housenka issues sync      # načte issue přiřazená tobě
housenka issues           # vypíše, kolik je nových, zařazených a skrytých
```

V prohlížeči totéž najdeš v **⚙ Nastavení → Issues**: tlačítko Zapnout, pole pro token a editor stavů.

**Token** vytvoříš v GitLabu v *Preferences → Access tokens* s oprávněním `read_api`. Housenka ho uloží do souboru `~/.housenka/gitlab-token` čitelného jen pro tebe. Do configu ani do `issues.yaml` nikdy nepatří a web ho nikdy nezobrazí zpět. Místo souboru můžeš použít proměnnou `HOUSENKA_GITLAB_TOKEN`, ta má přednost.

**Vypnutí:** smaž (nebo přejmenuj) `issues.yaml`, nebo v Nastavení → Issues klikni na Vypnout. Stavy, poznámky ani token se tím nesmažou a Zapnout je vrátí.

## Nastavení: `issues.yaml`

Rozšíření má vlastní soubor vedle `config.yaml`, aby config výkazů zůstal přehledný.

```yaml
host: gitlab.example.com  # GitLab, ze kterého se načítají issue; vynech = první z gitlab_hosts
sync_minutes: 15          # jak často webové rozhraní GitLab načítá
stale_days: 14            # karta se obarví, když na issue tolik dní nesáhneš

statuses:                 # tvoje stavy = sloupce nástěnky v tomto pořadí
  - { code: todo,    name: Na řadě,      color: blue }
  - { code: doing,   name: Rozpracováno, color: orange }
  - { code: waiting, name: Čeká,         color: purple }
  - { code: done,    name: Hotovo,       color: green, watch: false }
```

- `code` se ukládá k issue, `name` se zobrazuje. Barva je jedna z `red`, `orange`, `yellow`, `green`, `teal`, `cyan`, `blue`, `purple`, `pink` (odstín podle tématu), nebo `"#rrggbb"` v uvozovkách. Bez barvy se odvodí z kódu.
- `watch: false` vypne hlídání zapomenutých issue v daném stavu (typicky Hotovo).
- Upravuje se příkazem `housenka issues edit` (po uložení se zkontroluje), nebo v Nastavení → Issues. Tam se stavy přidávají, přejmenovávají, přesouvají a mažou. Přejmenování kódu přeřadí i issue v tom stavu, smazání stavu vrátí jeho issue mezi nová. Komentáře a rozložení souboru zůstanou, předchozí verze je v `issues.yaml.bak`.
- `housenka config check` zkontroluje i `issues.yaml`.

## Odkud se issue berou

Housenka se ptá GitLabu na **otevřená issue přiřazená tobě** (vlastníku tokenu):

- při `housenka issues sync` nebo klávese `r`,
- webový server každých `sync_minutes`,
- TUI při otevření přehledu, když je poslední načtení starší než `sync_minutes`.

U nového issue zjistí, **kdy ti bylo přiřazeno** (ze systémové poznámky „assigned to @ty“). Když ji nenajde, platí čas, kdy issue poprvé uviděla.

Issue, které ze seznamu zmizí, se zeptá zvlášť: zavřené dostane značku **zavřeno v GitLabu**, přeřazené na někoho jiného **už není na tobě**. Smazané issue a issue, ke kterému ztratíš přístup, se označí jako zavřené. Když se issue znovu otevře nebo vrátí na tebe, značka zmizí; skryté issue se pak zase ukáže. Název, štítky a termín se při každém načtení obnoví, tvůj stav, pořadí a poznámky nikdy.

Když se projekt přejmenuje nebo přesune, issue si stav, pořadí a poznámky vezme s sebou pod novou adresu. Po změně `host` se issue původního GitLabu nechají, jak jsou (skryješ je).

Načítá vždy jen jedno okno najednou, i když běží TUI i web. Chyba načítání (síť, neplatný token) se ukáže v hlavičce přehledu, data zůstanou. Když GitLab přesměruje na jinou adresu (třeba přihlašovací stránku), Housenka tam token neposílá a načtení skončí chybou.

## Barvy a repozitáře

Repozitář issue má barvu svého **kódu projektu** z configu výkazů. Repozitář s víc projekty a neznámý repozitář dostanou barvu odvozenou z cesty.

Z GitLabu můžou přijít issue i z repozitářů, které Housenka nezná (nejsou v `repos` configu výkazů), nebo z GitLabu, který není v `gitlab_hosts`. Takové issue se normálně zobrazí se značkou **repozitář není v housence** (filtr `is:unknown`), jen na něj zatím nejde vykázat. V prohlížeči ho detail issue nabídne rovnou přidat (**Přidat repozitář do housenky**); v terminálu ho doplň do `repos` přes `housenka config edit`.

## Zapomenutá issue

Karta zežloutne a dostane značku **beze mě N dní**, když se na issue `stale_days` dní nic nestalo: nezměnil se stav ani poznámka a nic se na něj nevykázalo. Hlídají se nová issue a stavy bez `watch: false`; zavřená, přeřazená a skrytá ne. Řazení **nejdéle beze mě** je dá nahoru.

## Ovládání

TUI otevře přehled klávesou `T` (stavový řádek výkazů ukazuje, kolik je nových issue), web odkazem **Issues** v horní liště. Klávesy jsou v obou stejné:

| Akce | Klávesy |
|---|---|
| výběr issue | `j`/`k` · ↑/↓, `gg`/`G` |
| jiný sloupec nástěnky | `h`/`l` · ←/→ |
| nová / nástěnka / skrytá | `1`/`2`/`3` |
| do předchozího / dalšího stavu (nové issue: `L` zařadí do prvního) | `H`/`L` |
| posunout ve sloupci (při vlastním řazení) | `J`/`K` |
| detail; v něm `1`–`9` nastaví stav | Enter · `s` |
| poznámky (v TUI `ctrl+s` uloží, Esc zruší) | `n` |
| vykázat práci na issue | `v` |
| otevřít v GitLabu | `o` · `gx` |
| skrýt / vrátit | `x` |
| filtr | `/` (Esc zruší) |
| další řazení: vlastní pořadí, přiřazeno nejdřív, nejdéle beze mě, termín, repozitář | `O` |
| seskupit podle repozitáře | `p` |
| načíst z GitLabu | `r` |
| zpět na výkazy | `T` · Esc |

V prohlížeči jdou karty přetahovat myší mezi sloupci i v rámci sloupce. Na úzkém terminálu ukáže TUI jeden sloupec a `h`/`l` přepínají.

### Filtr

Slova se kombinují (musí platit všechna), velikost písmen nehraje roli, `-` před slovem ho neguje:

| Slovo | Najde |
|---|---|
| `r:shop` | repozitáře, jejichž cesta obsahuje „shop“ |
| `p:SHOP` | issue, na která jde vykázat kód projektu začínající „SHOP“ |
| `l:bug` | štítek GitLabu obsahující „bug“ |
| `is:stale` | zapomenutá issue |
| `is:unknown` | repozitář nebo GitLab mimo Housenku |
| `is:closed`, `is:unassigned` | zavřená, přeřazená |
| `is:notes`, `is:due` | s poznámkou, s termínem |
| cokoli jiného | název, číslo, cesta, štítky a poznámky |

Příklad: `r:shop -is:closed košík`.

## Kde jsou data

| Soubor | Obsah |
|---|---|
| `~/.housenka/issues.yaml` | nastavení rozšíření (zapnuto, dokud existuje) |
| `~/.housenka/issues/issues.json` | issue z GitLabu s tvými stavy, pořadím a poznámkami |
| `~/.housenka/gitlab-token` | token GitLabu (jen pro tebe čitelný) |

TUI a web můžou běžet zároveň: změny z jednoho se v druhém projeví a poznámky upravené mezitím jinde se nepřepíšou bez varování.

## Demo

Ukázkový domov v [examples/demo](../examples/demo) má přehled zapnutý. Generátor dat k němu vyrábí i issue (rozpracovaná, čekající, zapomenutá, nová, z neznámého repozitáře):

```sh
HOUSENKA_HOME=examples/demo housenka      # pak T
HOUSENKA_HOME=examples/demo housenka web
```

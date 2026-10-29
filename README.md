# 🐛 Housenka

Aplikace pro vykazování práce do Bílého motýla (BM), která hlídá, aby kombinace issue, kódu projektu, činnosti a času dávala smysl. Ovládá se v terminálu (TUI) nebo v prohlížeči (`housenka web`); obě rozhraní umí totéž a sdílí data.

- Z odkazu na GitLab issue pozná repozitář a nabídne **jen kódy projektů povolené v configu**.
- U projektu nabídne **jen povolené činnosti** a předvybere výchozí.
- Čas přijímá jen po čtvrthodinách (`2,75`, `2.5`, `1:45`).
- Pro každý záznam vygeneruje odkaz, který otevře BM s předvyplněným formulářem.
- Statistiky za den, týden a měsíc, poměr produktivní / neproduktivní práce a týdenní souhrn pro teamový status.

## Instalace

Na Linuxu, macOS a ve WSL stačí jeden příkaz:

```sh
curl -fsSL https://raw.githubusercontent.com/just-tomdo/housenka/main/install.sh | sh
```

Stáhne poslední vydanou verzi pro tvůj systém, ověří její kontrolní součet a uloží ji do `~/.local/bin` (jiný adresář nastavíš přes `HOUSENKA_INSTALL_DIR`). Když ten adresář nemáš v `PATH`, poradí, jak ho přidat. Na novou verzi aktualizuješ stejným příkazem, nainstalovanou verzi vypíše `housenka version`.

Na Windows stáhni `housenka-windows-amd64.exe` z [Releases](https://github.com/just-tomdo/housenka/releases). Kdo má Go, může i `go install github.com/just-tomdo/housenka@latest` nebo z repozitáře `go build -o housenka .`.

## První spuštění

Bez terminálu: spusť `housenka web`. Když config ještě neexistuje, vytvoří se ze vzoru a v prohlížeči tě provede **průvodce nastavením** (Nastavení → Průvodce, víc v [Nastavení v prohlížeči](#nastavení-v-prohlížeči)).

V terminálu:

```sh
housenka config init      # vytvoří ~/.housenka/config.yaml se vzorem a komentáři
housenka config edit      # otevře config v editoru a po uložení ho ověří
housenka config check     # ověří config, vypíše všechny chyby najednou
housenka parse https://gitlab.example.com/clients/acme-web/-/work_items/636   # zkouška mapování
housenka                  # spustí TUI
housenka web              # nebo webové rozhraní v prohlížeči
```

Ve vzorovém configu je potřeba vyplnit hlavně `bm.base_url` (adresa editoru výkazu) a `bm.fields` (názvy polí formuláře, které se předvyplní – zjistíš je z odkazu, který editor sám vytvoří), dále kódy činností a projektů a mapování repozitářů.

Kódy činností z BM bývají nečitelné. Činnost proto může mít nepovinný krátký popisek `short`, který se v seznamu, formuláři i souhrnech zobrazí místo kódu. Do BM se dál posílá kód. Dvě činnosti nesmí mít stejný popisek, to hlídá `housenka config check`.

```yaml
activities:
  X7DEV01: { name: Vývoj, short: DEV, productive: true }
```

`config edit` použije editor z `$VISUAL` nebo `$EDITOR` (třeba `export EDITOR=nvim`), jinak první nalezený z `nano`, `nvim`, `vim`, `vi`. Když config po uložení obsahuje chyby, vypíše je a nabídne ho otevřít znovu.

Vše je v adresáři `~/.housenka/` (config a `entries/<rok>.json`). Jiné umístění nastavíš přes `HOUSENKA_HOME`, jiný config přes `--config`.

## Demo

Ukázka s vymyšlenými projekty a půlrokem vyplněných výkazů je v [examples/demo](examples/demo) – spustíš ji bez zásahu do vlastních dat:

```sh
HOUSENKA_HOME=examples/demo housenka
```

Podrobnosti a generování dat k dnešnímu dni v [examples/README.md](examples/README.md).

## Ovládání

Klávesy jsou vim-friendly, ale každá akce má i běžnou alternativu. `?` zobrazí kompletní nápovědu.

| Akce | Klávesy |
|---|---|
| pohyb | `j`/`k`, `gg`/`G`, `ctrl+d`/`ctrl+u` · šipky, Home/End, PgDn/PgUp |
| předchozí / další den, týden či měsíc | `h`/`l` · ←/→ |
| dnes | `t` |
| den / týden / měsíc | `1`/`2`/`3` · Tab |
| přidat / upravit | `a` (`o`, Insert) / `e` (Enter) |
| kopie záznamu na dnešek | `c` |
| zapamatovat / vložit kopii na zobrazený den | `yy` / `p` |
| smazat | `dd` · Delete |
| otevřít v BM / kopírovat odkaz do BM | `b` / `Y` |
| otevřít issue v prohlížeči | `i` · `gx` |
| souhrn issue (všechny výkazy, hodiny podle činností) | `I` (v souhrnu `i` otevře issue) |
| přepnout „odesláno“ | `s` · Space |
| týdenní souhrn pro status | `S` (v souhrnu `y` zkopíruje text, `m` Markdown) |
| filtr | `/` (Esc zruší) |
| konec | `q`, `:q`, `ctrl+c` |

Ve formuláři: Tab / Enter další pole, Shift+Tab předchozí pole, `h`/`l` nebo `j`/`k` vybírají kód a činnost. Popis je víceřádkový (Enter = nový řádek, Tab = dál). Na konci jsou tlačítka **Uložit** a **Zrušit** (Enter je stiskne); zkratky `ctrl+s` uloží a Esc zruší odkudkoli. Datum přijme `12. 10. 2026`, `12.10.`, `dnes`, `včera`, `-1` nebo cokoli ve formátu z `ui.date_format`. V poli Datum otevře `ctrl+k` kalendář: `h`/`l` den, `j`/`k` týden, `H`/`L` měsíc, `t` dnes, Enter vybere, Esc zavře.

Úprava už odeslaného záznamu ho označí jako neodeslaný – v BM je pak potřeba upravit i původní výkaz.

## Webové rozhraní

Pro ty, kdo nechtějí do terminálu, umí Housenka totéž v prohlížeči: přehled dne, týdne a měsíce, formulář s nabídkou povolených kódů a činností, odkazy do BM, souhrn issue i týdenní souhrn. Funguje i se stejnými klávesovými zkratkami jako TUI (`?` je vypíše).

```sh
housenka web              # spustí server a otevře prohlížeč, Ctrl+C ho ukončí
housenka web start        # spustí ho na pozadí (terminál pak můžeš zavřít)
housenka web status       # běží? na jaké adrese?
housenka web stop         # vypne server běžící na pozadí
```

Server na pozadí se nemůže „ztratit“. Svůj stavový soubor `~/.housenka/web.json` si hlídá a když zmizí, zapíše ho znovu. `status` a `stop` ho najdou i podle portu. Když server na požadavek o vypnutí nezareaguje, `stop` ukončí jeho proces. Když server vůbec neodpovídá, `status` to řekne a `housenka web stop --force` ho ukončí.

Server poslouchá jen na tomto počítači (`127.0.0.1`), ostatní v síti se k němu nedostanou. TUI a web můžou běžet zároveň, změny z jednoho se v druhém projeví. Záznamy z běhu na pozadí jdou do `~/.housenka/web.log`.

### Adresa bez portu

Aby stačilo napsat `http://housenka.localhost/` (nebo `http://housenka.local/`), nastav port 80 – v Nastavení → Obecné → Webové rozhraní, nebo v configu `web.port: 80` – a server restartuj (`housenka web stop` a `housenka web start`).

- **Windows a macOS**: funguje rovnou, jen port 80 nesmí používat jiný program (třeba IIS).
- **Linux a WSL**: porty pod 1024 smí běžně otevřít jen správce. Jednorázově je povol pro všechny programy:

  ```sh
  echo 'net.ipv4.ip_unprivileged_port_start=80' | sudo tee /etc/sysctl.d/50-housenka.conf
  sudo sysctl --system
  ```

  Ve WSL se port 80 předá do Windows stejně jako 8765, takže stránku otevřeš v prohlížeči ve Windows. Název `housenka.local` pak přidej do hosts ve Windows (`127.0.0.1 housenka.local`).

### Nastavení v prohlížeči

Celý config se dá upravit v sekci **⚙ Nastavení**, bez psaní YAML:

- **Průvodce** ukáže, které části configu ještě obsahují vzorové hodnoty, a odkáže na jejich úpravu. Vzorové činnosti, projekty a repozitáře pak smaže jedním tlačítkem.
- **Repozitáře**: stačí vložit odkaz na repozitář z GitLabu (klidně i odkaz na issue, adresu `.git` nebo `git@…`) nebo napsat `skupina/projekt`. Housenka nabídne pravidlo jen pro tento repozitář nebo pro celou skupinu, upozorní na existující pravidlo a neznámou doménu GitLabu přidá. Nový projekt jde založit rovnou ve stejném formuláři.
- **Projekty**: přehled kódů s barvou, povolenými a výchozí činností, repozitáři a počtem záznamů. Ve formuláři se vybírá barva štítku, povolené činnosti a repozitáře.
- **Činnosti**: kódy činností z BM, jejich krátké popisky a jestli jsou produktivní.
- **Obecné**: domény GitLabu, adresa a pole Bílého motýla (vyplní se samy z odkazu, který vytvoří editor BM), časové limity, web a vzhled TUI.
- **YAML**: celý soubor jako text pro pokročilé úpravy. Když je config rozbitý, web nabídne jen tuhle záložku, dokud ho neopravíš.

Přejmenování kódu se propíše do celého configu; uložené záznamy si nechají starý kód, tak jak byly odeslány do BM. Smazání se vždy potvrzuje a vypíše, co dalšího se změní. Každá změna se před uložením zkontroluje, komentáře v configu zůstanou a předchozí verze se uloží do `config.yaml.bak`.

Nastavení je v sekci `web` configu:

```yaml
web:
  port: 8765                      # nebo HOUSENKA_WEB_PORT=9000, nebo --port 9000
  hostnames: [housenka.localhost] # názvy, na kterých server odpovídá
  open_browser: true
  theme: auto                     # auto = světlé nebo tmavé podle systému
```

Výchozí adresa je `http://housenka.localhost:8765/`. Názvy končící na `.localhost` prohlížeče samy směrují na tento počítač, není potřeba nic nastavovat. Jiný název, třeba `housenka.local`, přidej do `hostnames` a do souboru hosts (na Windows `C:\Windows\System32\drivers\etc\hosts`, na Linuxu a macOS `/etc/hosts`) řádek `127.0.0.1 housenka.local`. Koncovka `.local` je vyhrazená pro mDNS, takže na některých systémech může první načtení zdržet. Když běží Housenka ve WSL, otevře se stránka v prohlížeči ve Windows; WSL2 ve výchozím nastavení předává adresy `localhost` do WSL automaticky.

Barevné téma se přepíná vpravo nahoře a prohlížeč si volbu pamatuje. K dispozici jsou světlá a tmavá Housenka, Tokyo Night (i Day), Catppuccin Mocha a Latte, Gruvbox tmavý a světlý a Ayu Dark. Nové téma je jeden soubor v [`internal/web/static/themes/`](internal/web/static/themes) – stačí zkopírovat existující, změnit název v hlavičce (`/* name: …; scheme: light|dark */`) a barvy.

## Podporované systémy

Linux, macOS, Windows a WSL. Prohlížeč a schránka se detekují automaticky (`xdg-open`, `open`, `wslview`, `rundll32`; `wl-copy`, `xclip`, `pbcopy`, `clip.exe`). Když schránkový nástroj chybí, kopíruje se přes terminál (OSC 52). Příkaz prohlížeče jde přepsat v `bm.open_command`.

Formát zobrazovaného data nastavíš v `ui.date_format`: `d`/`dd` den, `m`/`mm` měsíc, `yy`/`yyyy` rok, ostatní znaky se opíšou. Výchozí je `d. m. yyyy` (9. 10. 2026), dále třeba `dd.mm.yyyy` nebo `yyyy-mm-dd`.

Každý projekt má barevný štítek. Barvu si můžeš u projektu napevno zvolit klíčem `color`: pojmenovanou (`red`, `orange`, `yellow`, `green`, `teal`, `cyan`, `blue`, `purple`, `pink`), jejíž odstín se přizpůsobí theme, nebo přesnou `"#rrggbb"` (v uvozovkách). Projekty bez `color` dostanou barvu odvozenou z kódu, která se nemění.

Aplikace kreslí pozadí podle zvoleného theme (`ui.theme`). Kdo chce ponechat pozadí terminálu (třeba průhledné), nastaví `ui.background: false`.

---

<sub>🐛 Tuhle housenku jsem napsal já, Claude (AI od Anthropicu). Autor repa mi celou dobu koukal přes rameno. Zadával, škrtal, vracel rámečky, přidával mezery a nechal mě anonymizovat všechno, co šlo. Kód jsem psal já, o tom, co v něm zůstane, rozhodoval on. Kdyby něco nefungovalo, testy tvrdí, že to není možné. Kdyby přesto, je to pořád housenka a motýl se z ní teprve klube. 🦋</sub>

<sub>— Claude · vykázáno 0,00 h · činnost: neproduktivní (AI si do Bílého motýla vykazovat nesmí)</sub>

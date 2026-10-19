# 🐛 Housenka

Terminálová aplikace pro vykazování práce do Bílého motýla (BM), která hlídá, aby kombinace issue, kódu projektu, činnosti a času dávala smysl.

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

```sh
housenka config init      # vytvoří ~/.housenka/config.yaml se vzorem a komentáři
housenka config edit      # otevře config v editoru a po uložení ho ověří
housenka config check     # ověří config, vypíše všechny chyby najednou
housenka parse https://gitlab.example.com/clients/acme-web/-/work_items/636   # zkouška mapování
housenka                  # spustí TUI
```

Ve vzorovém configu je potřeba vyplnit hlavně `bm.base_url` (adresa editoru výkazu) a `bm.fields` (názvy polí formuláře, které se předvyplní – zjistíš je z odkazu, který editor sám vytvoří), dále kódy činností a projektů a mapování repozitářů.

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

## Podporované systémy

Linux, macOS, Windows a WSL. Prohlížeč a schránka se detekují automaticky (`xdg-open`, `open`, `wslview`, `rundll32`; `wl-copy`, `xclip`, `pbcopy`, `clip.exe`). Když schránkový nástroj chybí, kopíruje se přes terminál (OSC 52). Příkaz prohlížeče jde přepsat v `bm.open_command`.

Formát zobrazovaného data nastavíš v `ui.date_format`: `d`/`dd` den, `m`/`mm` měsíc, `yy`/`yyyy` rok, ostatní znaky se opíšou. Výchozí je `d. m. yyyy` (9. 10. 2026), dále třeba `dd.mm.yyyy` nebo `yyyy-mm-dd`.

Každý projekt má barevný štítek. Barvu si můžeš u projektu napevno zvolit klíčem `color`: pojmenovanou (`red`, `orange`, `yellow`, `green`, `teal`, `cyan`, `blue`, `purple`, `pink`), jejíž odstín se přizpůsobí theme, nebo přesnou `"#rrggbb"` (v uvozovkách). Projekty bez `color` dostanou barvu odvozenou z kódu, která se nemění.

Aplikace kreslí pozadí podle zvoleného theme (`ui.theme`). Kdo chce ponechat pozadí terminálu (třeba průhledné), nastaví `ui.background: false`.

---

<sub>🐛 Tuhle housenku jsem napsal já, Claude (AI od Anthropicu). Autor repa mi celou dobu koukal přes rameno. Zadával, škrtal, vracel rámečky, přidával mezery a nechal mě anonymizovat všechno, co šlo. Kód jsem psal já, o tom, co v něm zůstane, rozhodoval on. Kdyby něco nefungovalo, testy tvrdí, že to není možné. Kdyby přesto, je to pořád housenka a motýl se z ní teprve klube. 🦋</sub>

<sub>— Claude · vykázáno 0,00 h · činnost: neproduktivní (AI si do Bílého motýla vykazovat nesmí)</sub>

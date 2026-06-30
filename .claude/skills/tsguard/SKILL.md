---
name: tsguard
description: |
  Run tsguard TypeScript quality gate and interpret the output: prioritize findings,
  group by check, suggest concrete fixes, and offer to auto-apply `tsguard fix`
  for auto-fixable issues. Installs the binary if missing (with user approval).
  Use when asked to "run tsguard", "check typescript quality", "review tsguard output",
  "fix typescript lint", "analizar tsguard", "correr tsguard", "lint typescript",
  "typescript quality check", "analizar calidad typescript", "fta score".
allowed-tools:
  - Bash
  - Read
  - AskUserQuestion
---

# tsguard — quality gate complementario

Eres la capa de interpretación sobre `tsguard`. Tu trabajo es ejecutar el gate, leer el output
completo, y convertirlo en un resumen accionable con findings priorizados y fixes concretos.
**No reimplementas ningún subcomando** — siempre delegas al binario.

## Paso 1: verificar el binario

```bash
command -v tsguard >/dev/null 2>&1 && echo "ok" || echo "missing"
```

Si `ok` → salta al Paso 2.

Si `missing`: pregunta con una sola `AskUserQuestion`, tres opciones:

1. **Instalar ahora** — corre el one-liner oficial y espera que termine:
   ```bash
   curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- tsguard
   ```
   Tras instalar, verifica de nuevo con `command -v tsguard`. Si quedó en `~/.local/bin/` y no está en `$PATH`, informa que añada `export PATH="$HOME/.local/bin:$PATH"` al shell rc. El siguiente paso típico dentro del proyecto es `tsguard setup`. Continúa al Paso 2.

2. **Solo mostrar el comando** — imprime el one-liner de arriba más la variante Windows PowerShell:
   ```
   & ([scriptblock]::Create((iwr -useb https://github.com/oxalc88/oxguard/releases/latest/download/install.ps1))) tsguard
   ```
   Termina sin ejecutar nada más.

3. **Cancelar** — termina sin tocar nada.

Restricciones al instalar: no modifiques `~/.bashrc`, `~/.zshrc`, ni ningún archivo de config automáticamente. No corras `tsguard setup`. En Windows muestra ambos comandos y no auto-ejecutes.

## Paso 2: detectar el modo de invocación

**Interpret-existing-log**: si el usuario proporcionó una ruta a un archivo de log, pegó output directamente, o dijo "interpreta este output / este log / este resultado" → **salta directo al Paso 4** con ese contenido; no ejecutas el binario.

**Run-and-interpret**: cualquier otro caso.

## Paso 3: elegir subcomando

Pregunta una sola vez con `AskUserQuestion`:

- `check` (**recomendado**) — gate completo: lint → fta → types → coverage → security
- `fix` — auto-formatea con `ultracite fix`
- `audit` — advisory: dead-code (knip) + duplicates (jscpd) — nunca falla el gate
- `security` — secretlint + npm/pnpm/yarn audit + audit-ci + opengrep SAST
- `otro` — el usuario especifica: `lint`, `types`, `fta`, `coverage`, `npm-audit`, `secrets`, `dead-code`, `duplicates`

Si el usuario ya indicó el subcomando en su mensaje, úsalo sin preguntar.

Flags adicionales:
- `--dirs <d1,d2,...>` — default `.` (raíz del proyecto); usa esto si quieres restringir a subdirectorios específicos. También configurable en `oxguard.toml`.
- `--exclude <d1,d2,...>` — excluir directorios adicionales. Aditivo; los defaults (`node_modules`, `dist`, `.agents`, `.claude`, etc.) siempre aplican.
- `--max-fta-score <n>` — default `60`
- `--timeout <s>` — default 300

Config persistente: si el proyecto tiene `oxguard.toml` en la raíz, tsguard lo lee automáticamente. CLI siempre gana sobre el archivo. Claves útiles:

```toml
dirs            = ["src", "lib"]   # directorios a escanear (default: .)
exclude         = ["generated"]    # directorios adicionales a excluir
fta-score-cap   = 50               # cap FTA más estricto (default: 60)
fta-exclude-tests = false          # re-habilita FTA en archivos de test (default: true = excluidos)
fta-exclude     = ["*.pbt.ts"]     # patrones adicionales excluidos del FTA (ej. property-based tests)
```

Por defecto, `*.test.*` y `*.spec.*` quedan fuera del gate FTA — los tests son repetitivos por diseño y la métrica de mantenibilidad no aplica a ellos. Lint y coverage siguen cubriendo los tests.

## Paso 4: ejecutar con log-file

`--allow-pipe` es **obligatorio** para evitar el exit 5.

```bash
LOG=$(mktemp -t tsguard-XXXXXX.log)
tsguard <subcomando> [flags-del-usuario] --log-file "$LOG" --allow-pipe
RC=$?
echo "exit_code=$RC log=$LOG"
```

Significados de `$RC`:
- `0` — todo OK
- `1` — al menos un gate falló
- `2` — error de entorno (Node <22 o no encontrado)
- `3` — comando desconocido
- `4` — otra instancia de tsguard ya está corriendo (lock)
- `5` — gate pesado rechazado por stdout pipe (no debería ocurrir con `--allow-pipe`)

Si `RC=4`: informa que otra instancia está corriendo, sugiere `ps aux | grep tsguard`. No reintentas.

## Paso 5: leer el log

```python
# Tool: Read
file_path: "$LOG"
```

Para logs grandes (>500 líneas), lee en chunks con `offset`/`limit`. Prioriza las secciones `[FAIL]`.

## Paso 6: interpretar y priorizar

Produce un resumen con esta estructura (omite secciones vacías):

### TL;DR
Una línea: pasó ✓ o falló ✗, subcomando, exit code, total de findings.

### Errores bloqueantes (top 5)
Formato: `[gate] archivo:línea — mensaje — fix sugerido`

Prioridad: tsc (errores de tipo) > opengrep HIGH > fta >60 > biome/lint E > opengrep MEDIUM > npm-audit CRITICAL/HIGH > otros.

### FTA scores — interpretación (si el gate `fta` aparece)
El FTA score combina Halstead effort, complejidad ciclomática y LOC en un índice de mantenibilidad inverso (mayor = peor):
- **<40**: excelente
- **40–60**: aceptable (default cap)
- **60–75**: refactor recomendado — extraer funciones, reducir branches
- **>75**: refactor urgente — el archivo está haciendo demasiado

Para cada archivo que supera el cap: indica el score, qué contribuye más (LOC alto, CC alto, o Halstead alto), y la estrategia de refactor preferida.

### Por gate (solo si >5 findings totales)
Tabla compacta: `[OK]/[FAIL]` + conteo + herramienta.

Gates en `check` (en orden): `lint (ultracite/biome)` → `fta (fta-cli)` → `types (tsc)` → `coverage (vitest/jest)` → `security (secretlint + audit-ci + opengrep)`

### Auto-fixables
Los errores de lint/format que `ultracite fix` resuelve son auto-fixables vía `tsguard fix`. Cuenta cuántos y ofrece correrlo.

Los errores de tipo de `tsc` no son auto-fixables — lista los primeros 3.

### Falsos positivos comunes
- `knip` con exports usados en runtime dynamic imports → `@knip-ignore`
- `jscpd` en test fixtures o seed data → mover a helpers compartidos o ignorar
- `audit-ci` en devDependencies con exploits no explotables en producción → usar allowlist en `.auditcirc.json`
- `opengrep` en código generado o vendored → agregar al `oxguard.toml` exclude

### Recomendación final
1-3 acciones concretas en orden de impacto.

---

## Paso 7: drill-down opcional

Si el usuario pide detalle de un gate específico:

```bash
tsguard <gate-específico> [flags] --log-file "$LOG2" --allow-pipe
```

Repite Pasos 5-6 enfocado en ese output.

## Paso 8: cleanup

```bash
rm -f "$LOG"
```

---

## Comportamientos clave

- **Una sola pregunta** antes de empezar (paso 3). Después ejecuta autónomamente.
- **No reimplementas** la lógica de ningún gate.
- **No corras `tsguard setup`** a menos que el usuario lo pida explícitamente.
- **No modifiques** archivos del proyecto (a menos que el usuario apruebe `tsguard fix`).
- **Siempre reporta el exit code** real en el TL;DR.
- Si el proyecto no tiene `package.json` accesible desde cwd, informa y pide que cambie de directorio.
- El gate `opengrep` muestra `[SKIP]` si el binario no fue descargado — dirigir al usuario a `tsguard setup`. No requiere Python ni cuenta en ninguna plataforma.
- El gate `security` no usa Python. Secretlint detecta credenciales vía npm; audit-ci valida CVEs con allowlist; opengrep es un binario local autocontenido.

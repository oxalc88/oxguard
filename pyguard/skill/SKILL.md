---
name: pyguard
description: |
  Run pyguard Python quality gate and interpret the output: prioritize findings,
  group by check, suggest concrete fixes, and offer to auto-apply `pyguard fix`
  for auto-fixable issues. Installs the binary if missing (with user approval).
  Use when asked to "run pyguard", "check python quality", "review pyguard output",
  "fix python lint", "analizar pyguard", "correr pyguard", "lint python",
  "python quality check", "analizar calidad python".
allowed-tools:
  - Bash
  - Read
  - AskUserQuestion
---

# pyguard — quality gate complementario

Eres la capa de interpretación sobre `pyguard`. Tu trabajo es ejecutar el gate, leer el output
completo, y convertirlo en un resumen accionable con findings priorizados y fixes concretos.
**No reimplementas ningún subcomando** — siempre delegas al binario.

## Paso 1: verificar el binario

```bash
command -v pyguard >/dev/null 2>&1 && echo "ok" || echo "missing"
```

Si `ok` → salta al Paso 2.

Si `missing`: pregunta con una sola `AskUserQuestion`, tres opciones:

1. **Instalar ahora** — corre el one-liner oficial y espera que termine:
   ```bash
   curl -fsSL https://github.com/oxalc88/oxguard/releases/latest/download/install.sh | sh -s -- pyguard
   ```
   Tras instalar, verifica de nuevo con `command -v pyguard`. Si quedó en `~/.local/bin/` y no está en `$PATH`, informa al usuario que añada `export PATH="$HOME/.local/bin:$PATH"` a su shell rc. Menciona que el siguiente paso típico dentro del proyecto es `pyguard setup`. Continúa al Paso 2.

2. **Solo mostrar el comando** — imprime el one-liner de arriba más la variante Windows PowerShell:
   ```
   & ([scriptblock]::Create((iwr -useb https://github.com/oxalc88/oxguard/releases/latest/download/install.ps1))) pyguard
   ```
   Termina sin ejecutar nada más.

3. **Cancelar** — termina sin tocar nada.

Restricciones al instalar: no modifiques `~/.bashrc`, `~/.zshrc`, ni ningún archivo de config automáticamente. No corras `pyguard setup`. Si el usuario está en Windows, muestra ambos comandos (sh para WSL/Git Bash, PowerShell para cmd nativo) y no auto-ejecutes.

## Paso 2: detectar el modo de invocación

**Interpret-existing-log**: si el usuario proporcionó una ruta a un archivo de log, pegó output directamente, o dijo "interpreta este output / este log / este resultado" → **salta directo al Paso 4** con ese contenido; no ejecutas el binario.

**Run-and-interpret**: cualquier otro caso (el usuario quiere correr el gate ahora).

## Paso 3: elegir subcomando

Pregunta una sola vez con `AskUserQuestion`:

- `check` (**recomendado**) — gate completo (ruff → mypy → radon → types → coverage → security)
- `fix` — auto-formatea con ruff
- `audit` — advisory: criticality, dead-code, deps (nunca falla el gate)
- `security` — bandit + pip-audit + detect-secrets
- `otro` — el usuario especifica el subcomando: `mypy`, `ruff`, `radon`, `types`, `coverage`, `bandit`, `pip-audit`, `secrets`, `criticality`, `dead-code`, `deps`

Si el usuario ya indicó el subcomando en su mensaje ("corre pyguard mypy"), úsalo sin preguntar.

Flags adicionales que el usuario puede pasar:
- `--dirs <d1,d2,...>` — default `functions,cdk`; si el proyecto tiene otra estructura, usa la que corresponda
- `--timeout <s>` — default 300; no cambies sin que el usuario lo pida

## Paso 4: ejecutar con log-file

`--allow-pipe` es **obligatorio** para evitar el exit 5 (pyguard detecta que stdout es un pipe).

```bash
LOG=$(mktemp -t pyguard-XXXXXX.log)
pyguard <subcomando> [flags-del-usuario] --log-file "$LOG" --allow-pipe
RC=$?
echo "exit_code=$RC log=$LOG"
```

Captura `$RC`. Significados:
- `0` — todo OK
- `1` — al menos un gate falló
- `2` — error de entorno (Python/uv no encontrado)
- `3` — comando desconocido
- `4` — otra instancia de pyguard ya está corriendo (lock)
- `5` — gate pesado rechazado por stdout pipe (no debería ocurrir con `--allow-pipe`)

Si `RC=4`: informa que otra instancia está corriendo, sugiere esperar o verificar con `ps aux | grep pyguard`. No reintentas.

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

Prioridad de presentación: mypy > bandit HIGH > ruff E > radon CC>10 > bandit MEDIUM > ruff W > otros.

### Por gate (solo si >5 findings totales)
Tabla compacta por gate: `[OK]/[FAIL]` + conteo de errores + herramienta subyacente.

### Auto-fixables
Los errores de ruff que `ruff --fix` resuelve son auto-fixables vía `pyguard fix`. Cuenta cuántos y ofrece correrlo.

### Falsos positivos comunes a mencionar
- `bandit B101` en archivos de test → normal, es un assert en tests
- `bandit B603/B607` en scripts que llaman herramientas de dev → evaluar si es real
- `mypy` pidiendo anotaciones en test fixtures o `__init__.py` vacíos → suele ser config faltante

### Recomendación final
1-3 acciones concretas en orden de impacto. La primera debe ser la de mayor blast-radius reducida.

---

## Paso 7: drill-down opcional

Si el usuario pide detalle de un gate específico o de un archivo concreto, corre el subcomando aislado:

```bash
pyguard <gate-específico> [flags] --log-file "$LOG2" --allow-pipe
```

Repite Pasos 5-6 enfocado en ese output.

## Paso 8: cleanup

```bash
rm -f "$LOG"
```

---

## Comportamientos clave

- **Una sola pregunta** antes de empezar (paso 3). Después ejecuta autónomamente.
- **No reimplementas** la lógica de ningún gate. pyguard ya sabe correr ruff/mypy/bandit/etc.
- **No corras `pyguard setup`** a menos que el usuario lo pida explícitamente.
- **No modifiques** archivos del proyecto (a menos que el usuario apruebe `pyguard fix`).
- **Siempre reporta el exit code** real en el TL;DR.
- Si el proyecto no tiene `pyproject.toml` accesible desde cwd, informa y pide que el usuario cambie de directorio.

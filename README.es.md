# apollo2punkt

[English](README.md)

Importa los juegos de un `apps.json` de Vibepollo, Apollo o Sunshine a la biblioteca de
[Punktfunk](https://github.com/punktfunk/punktfunk) del mismo PC con Windows. El comando de cada
juego se copia tal cual, así que los juegos que se lanzan con scripts, PsExec, Steam o su propio
`.exe` se siguen lanzando igual.

## Uso

1. Descarga `apollo2punkt.exe` desde [Releases](../../releases).
2. Comprueba que el host de Punktfunk está arrancado en el PC.
3. Clic derecho en `apollo2punkt.exe` → **Ejecutar como administrador**. Lo necesita para leer
   la clave de gestión del host en `%ProgramData%\punktfunk\mgmt-token`.
4. Busca el `apps.json` solo, enseña lo que va a importar y pregunta qué juegos importar
   (`1,3-5`, o Enter para todos).

El `.exe` no está firmado: Windows SmartScreen puede avisar la primera vez. Pulsa **Más
información → Ejecutar de todas formas**, o compílalo tú (abajo).

### Opciones

Desde una terminal de administrador:

| Opción | Qué hace |
| --- | --- |
| `-apps RUTA` | usa ese `apps.json` |
| `-export prueba.json` | no toca Punktfunk: guarda lo que enviaría, para revisarlo |
| `-update` | sobrescribe los juegos que ya importó |
| `-undo` | borra de Punktfunk todo lo que importó este programa |
| `-steam` | importa también los juegos de Steam que ya añade el plugin de Steam |
| `-no-art` | no busca carátulas |
| `-yes` | todo sin preguntar |

Crea `apollo2punkt-state.json` junto al `.exe`. Consérvalo para `-update` y `-undo`.

## Qué hace con cada app

- **Dónde busca el `apps.json`:** junto al `.exe`, luego en
  `Program Files\{Vibepollo,Apollo,Sunshine}\config\` y en `X:\{Vibepollo,Apollo,Sunshine}\config\`
  de cada unidad.
- **Lanzamiento:** conserva el comando, los comandos *detached*, el directorio de trabajo y los
  comandos de preparación y deshacer.
- **Detección**, para que Punktfunk sepa cuándo se cierra el juego:
  - `steam://rungameid/N` → el juego N de Steam;
  - un `.exe` en el comando → ese exe y su carpeta. Se saltan los intermediarios (PowerShell,
    PsExec, `cmd`, `steam.exe`…), así que un `launch.ps1 "D:\Juego\juego.exe"` vigila `juego.exe`;
  - un `.bat` → su carpeta;
  - nada → se importa igual, con un aviso.
- **Duplicados de Steam:** si el plugin de Steam ya añade el juego (`steam:N` en la biblioteca),
  se omite. Con `-steam` se importa igualmente.
- **Carátula:** si Vibepollo/Apollo la guardó como `igdb_<id>.png`, se usan la misma portada y
  fondo de IGDB. Las imágenes locales de otro tipo no se copian: elígela luego en Punktfunk.
- **Se omiten:** Desktop, Remote Input y Remote Monitor (no lanzan nada; Punktfunk trae su propio
  Escritorio) y Steam Big Picture (lo añade el plugin de Steam).
- Las opciones propias de Vibepollo (Lossless Scaling, `config-overrides`…) no existen en
  Punktfunk; avisa de las que tenías activas.

## Compilar

Con [Go](https://go.dev):

```sh
go test ./...
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o apollo2punkt.exe .
```

## Licencia

MIT o Apache-2.0, a tu elección.

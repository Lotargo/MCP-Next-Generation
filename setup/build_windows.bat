@echo off
setlocal
echo Building Go binaries for Windows...

:: 1. Определяем корневую директорию проекта.
set "PROJECT_ROOT=%~dp0.."

:: 2. Переходим в корневую директорию проекта.
pushd "%PROJECT_ROOT%"

:: ========================= НАЧАЛО БЛОКА АВТО-КОНФИГУРАЦИИ GO.WORK =========================
echo.
echo [A] Re-initializing Go workspace to match current project structure...

:: Удаляем старые файлы, чтобы избежать конфликтов
if exist "go.work" del "go.work"
if exist "go.work.sum" del "go.work.sum"

:: Создаем новый пустой go.work
go work init
if %errorlevel% neq 0 (
    echo Failed to initialize go workspace.
    popd
    exit /b 1
)

:: Ищем ВСЕ файлы go.mod рекурсивно и добавляем их родительские папки в go.work
echo [B] Automatically discovering and adding Go modules...
for /r . %%f in (go.mod) do (
    echo      - Found module, adding: %%~dpf
    :: КЛЮЧЕВОЕ ИЗМЕНЕНИЕ: Добавляем точку "." после %%~dpf, чтобы избежать проблемы с "экранированной кавычкой"
    go work use "%%~dpf."
)
echo.
:: ========================== КОНЕЦ БЛОКА АВТО-КОНФИГУРАЦИИ ===========================

:: 3. Устанавливаем переменные окружения для Go
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0

:: 4. Создаем папку bin
set "BIN_DIR=.\MCP-NG\bin"
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"

:: 5. Компилируем Meta-Server. Теперь go.work всегда будет актуальным.
echo Building Meta-Server...
go build -o "%BIN_DIR%\server.exe" .\MCP-NG\server\cmd\server
if %errorlevel% neq 0 (
    echo Failed to build Meta-Server.
    popd
    exit /b 1
)

:: 6. Компилируем все существующие Go утилиты
echo Building Go tools...
for /d %%d in (.\MCP-NG\tools\go\*) do (
    echo Building %%~nd...
    go build -o "%BIN_DIR%\%%~nd.exe" .\%%d
    if %errorlevel% neq 0 (
        echo Failed to build tool %%~nd.
        popd
        exit /b 1
    )
)

:: 7. Устанавливаем зависимости для Python инструментов
echo Installing dependencies for Python tools...
for /d %%d in (.\MCP-NG\tools\python\*) do (
    if exist "%%d\pyproject.toml" (
        echo Installing dependencies for %%~nd...
        pushd "%%d"
        poetry install
        if %errorlevel% neq 0 (
            echo Failed to install dependencies for %%~nd.
            popd
            popd
            exit /b 1
        )
        popd
    )
)

echo.
echo Windows build complete. Binaries are in %PROJECT_ROOT%\%BIN_DIR%

:: 8. Возвращаемся в исходную директорию
popd
endlocal

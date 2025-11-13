@echo off
echo Building FDM API...
go build -o fdm-api.exe
if %ERRORLEVEL% == 0 (
    echo Build successful! Executable created: fdm-api.exe
) else (
    echo Build failed!
)
pause

param(
    [Parameter(Mandatory = $true)]
    [string]$Version,

    [Parameter(Mandatory = $true)]
    [string]$BuildCommit,

    [Parameter(Mandatory = $true)]
    [string]$BuildDate,

    [string]$OutputDirectory,

    [string]$RuntimeLock
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$rootDirectory = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $rootDirectory "dist/portable/$Version"
}
if ([string]::IsNullOrWhiteSpace($RuntimeLock)) {
    $RuntimeLock = Join-Path $rootDirectory "packaging/windows/runtime.lock.json"
}

$outputPath = [System.IO.Path]::GetFullPath($OutputDirectory)
$lockPath = [System.IO.Path]::GetFullPath($RuntimeLock)
$sevenZip = (Get-Command 7z.exe -ErrorAction Stop).Source
$runtime = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json
$temporaryPath = Join-Path ([System.IO.Path]::GetTempPath()) ("doc7-portable-" + [System.Guid]::NewGuid().ToString("N"))
$downloadPath = Join-Path $temporaryPath "downloads"
$stagePath = Join-Path $temporaryPath "stage"
$packagePath = Join-Path $stagePath "doc7"

if ($Version -notmatch "^[A-Za-z0-9._-]+$") {
    throw "version may only contain letters, numbers, dots, underscores, and hyphens"
}

function Assert-Sha256 {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Expected
    )

    $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $Expected.ToLowerInvariant()) {
        throw "SHA-256 mismatch for $Path`: expected $Expected, got $actual"
    }
}

function Invoke-VerifiedDownload {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Url,

        [Parameter(Mandatory = $true)]
        [string]$Destination,

        [Parameter(Mandatory = $true)]
        [string]$Sha256
    )

    $lastError = $null
    for ($attempt = 1; $attempt -le 5; $attempt++) {
        try {
            Invoke-WebRequest -Uri $Url -OutFile $Destination
            $lastError = $null
            break
        }
        catch {
            $lastError = $_
            if ($attempt -lt 5) {
                Start-Sleep -Seconds ([Math]::Pow(2, $attempt - 1))
            }
        }
    }
    if ($null -ne $lastError) {
        throw $lastError
    }
    Assert-Sha256 -Path $Destination -Expected $Sha256
}

function Invoke-SevenZip {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    & $sevenZip @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "7-Zip failed with exit code $LASTEXITCODE"
    }
}

try {
    if (Test-Path -LiteralPath $outputPath) {
        throw "portable release output already exists: $outputPath"
    }

    New-Item -ItemType Directory -Path $downloadPath, $packagePath, $outputPath -Force | Out-Null

    $libreOfficeArchive = Join-Path $downloadPath "LibreOfficePortable.paf.exe"
    $muPdfArchive = Join-Path $downloadPath "mupdf-windows.zip"
    $muPdfSource = Join-Path $downloadPath "mupdf-$($runtime.mupdf.version)-source.tar.gz"

    Invoke-VerifiedDownload -Url $runtime.libreoffice.archive_url -Destination $libreOfficeArchive -Sha256 $runtime.libreoffice.archive_sha256
    Invoke-VerifiedDownload -Url $runtime.mupdf.archive_url -Destination $muPdfArchive -Sha256 $runtime.mupdf.archive_sha256
    Invoke-VerifiedDownload -Url $runtime.mupdf.source_url -Destination $muPdfSource -Sha256 $runtime.mupdf.source_sha256

    $libreOfficeExtract = Join-Path $temporaryPath "libreoffice"
    $muPdfExtract = Join-Path $temporaryPath "mupdf"
    New-Item -ItemType Directory -Path $libreOfficeExtract, $muPdfExtract -Force | Out-Null
    Invoke-SevenZip -Arguments @("x", "-y", "-o$libreOfficeExtract", $libreOfficeArchive)
    Expand-Archive -LiteralPath $muPdfArchive -DestinationPath $muPdfExtract -Force

    $soffice = Join-Path $libreOfficeExtract "App/libreoffice/program/soffice.exe"
    $muPdfRoot = Join-Path $muPdfExtract "mupdf-$($runtime.mupdf.version)-windows"
    $mutool = Join-Path $muPdfRoot "mutool.exe"
    if (-not (Test-Path -LiteralPath $soffice)) {
        throw "LibreOffice archive does not contain App/libreoffice/program/soffice.exe"
    }
    if (-not (Test-Path -LiteralPath $mutool)) {
        throw "MuPDF archive does not contain mutool.exe"
    }

    $ldflags = @(
        "-s",
        "-w",
        "-X github.com/magicrew/doc7/internal/cli.buildVersion=$Version",
        "-X github.com/magicrew/doc7/internal/cli.buildCommit=$BuildCommit",
        "-X github.com/magicrew/doc7/internal/cli.buildDate=$BuildDate"
    ) -join " "

    Push-Location $rootDirectory
    try {
        $env:CGO_ENABLED = "0"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        $goBuildArguments = @(
            "build",
            "-trimpath",
            "-ldflags",
            $ldflags,
            "-o",
            (Join-Path $packagePath "doc7.exe"),
            "./cmd/doc7"
        )
        & go @goBuildArguments
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }

    Copy-Item -LiteralPath (Join-Path $rootDirectory "LICENSE") -Destination $packagePath
    Copy-Item -LiteralPath (Join-Path $rootDirectory "README.md") -Destination $packagePath
    Copy-Item -LiteralPath (Join-Path $rootDirectory "README.zh-CN.md") -Destination $packagePath
    Copy-Item -Path (Join-Path $rootDirectory "packaging/windows/*") -Destination $packagePath -Recurse
    New-Item -ItemType Directory -Path (Join-Path $packagePath "examples") -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $rootDirectory "examples/visual-report") -Destination (Join-Path $packagePath "examples") -Recurse
    Copy-Item -LiteralPath (Join-Path $rootDirectory "examples/format-parity") -Destination (Join-Path $packagePath "examples") -Recurse

    $libreOfficeDestination = Join-Path $packagePath "tools/LibreOfficePortable"
    $muPdfDestination = Join-Path $packagePath "tools/mupdf"
    New-Item -ItemType Directory -Path $libreOfficeDestination, $muPdfDestination -Force | Out-Null
    Get-ChildItem -LiteralPath $libreOfficeExtract | Where-Object {
        $_.Name -ne '$PLUGINSDIR'
    } | Copy-Item -Destination $libreOfficeDestination -Recurse
    Copy-Item -LiteralPath $mutool -Destination $muPdfDestination
    Copy-Item -LiteralPath (Join-Path $muPdfRoot "COPYING.txt") -Destination $muPdfDestination
    Copy-Item -LiteralPath (Join-Path $muPdfRoot "README.txt") -Destination $muPdfDestination
    Copy-Item -LiteralPath (Join-Path $muPdfRoot "CHANGES.txt") -Destination $muPdfDestination
    Copy-Item -LiteralPath (Join-Path $muPdfRoot "CONTRIBUTORS.txt") -Destination $muPdfDestination

    $runtimeManifest = [ordered]@{
        schema_version = 1
        doc7 = [ordered]@{
            version = $Version
            commit = $BuildCommit
            build_date = $BuildDate
        }
        libreoffice = $runtime.libreoffice
        mupdf = $runtime.mupdf
    }
    $manifestPath = Join-Path $outputPath "runtime-manifest.json"
    $runtimeManifest | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $manifestPath -Encoding utf8NoBOM
    Copy-Item -LiteralPath $manifestPath -Destination $packagePath

    $sbom = [ordered]@{
        bomFormat = "CycloneDX"
        specVersion = "1.6"
        serialNumber = "urn:uuid:$([System.Guid]::NewGuid())"
        version = 1
        metadata = [ordered]@{
            timestamp = $BuildDate
            component = [ordered]@{
                type = "application"
                name = "doc7-windows-portable"
                version = $Version
            }
        }
        components = @(
            [ordered]@{ type = "application"; name = "doc7"; version = $Version; purl = "pkg:github/magicrew/doc7@$Version" },
            [ordered]@{ type = "application"; name = $runtime.libreoffice.name; version = $runtime.libreoffice.version; licenses = @([ordered]@{ license = [ordered]@{ id = $runtime.libreoffice.license } }) },
            [ordered]@{ type = "application"; name = $runtime.mupdf.name; version = $runtime.mupdf.version; licenses = @([ordered]@{ license = [ordered]@{ id = $runtime.mupdf.license } }) }
        )
    }
    $sbomPath = Join-Path $outputPath "portable-sbom.cdx.json"
    $sbom | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $sbomPath -Encoding utf8NoBOM

    $longestPath = 0
    Get-ChildItem -LiteralPath $packagePath -Recurse -File | ForEach-Object {
        $relativePath = [System.IO.Path]::GetRelativePath($stagePath, $_.FullName)
        $longestPath = [Math]::Max($longestPath, $relativePath.Length)
    }
    if ($longestPath -gt 200) {
        throw "portable archive contains a path longer than 200 characters: $longestPath"
    }

    $credentialPattern = "gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----"
    $ipv4Pattern = "(?<!\d)(?:\d{1,3}\.){3}\d{1,3}(?!\d)"
    # Runtime archives are verified by SHA-256 before extraction; scan project-owned text here.
    $toolsPathPrefix = (Join-Path $packagePath "tools") + [System.IO.Path]::DirectorySeparatorChar
    Get-ChildItem -LiteralPath $packagePath -Recurse -File | Where-Object {
        -not $_.FullName.StartsWith($toolsPathPrefix, [System.StringComparison]::OrdinalIgnoreCase) -and
        $_.Extension -in @(".md", ".txt", ".json", ".bat", ".ps1", ".yaml", ".yml")
    } | ForEach-Object {
        $content = Get-Content -LiteralPath $_.FullName -Raw
        if ($content -match $credentialPattern) {
            throw "portable package contains a credential or private-key pattern: $($_.FullName)"
        }
        foreach ($match in [regex]::Matches($content, $ipv4Pattern)) {
            $address = $null
            if (-not [System.Net.IPAddress]::TryParse($match.Value, [ref]$address)) {
                continue
            }
            if ($address.AddressFamily -eq [System.Net.Sockets.AddressFamily]::InterNetwork -and
                $match.Value -notin @("0.0.0.0", "127.0.0.1")) {
                throw "portable package contains a non-local IPv4 address: $($match.Value) in $($_.FullName)"
            }
        }
    }

    $archiveName = "doc7_${Version}_windows_amd64_portable.zip"
    $archivePath = Join-Path $outputPath $archiveName
    Push-Location $stagePath
    try {
        Invoke-SevenZip -Arguments @("a", "-tzip", "-mx=0", $archivePath, "doc7")
    }
    finally {
        Pop-Location
    }

    Copy-Item -LiteralPath $muPdfSource -Destination $outputPath

    $checksumPath = Join-Path $outputPath "checksums.txt"
    Get-ChildItem -LiteralPath $outputPath -File | Where-Object {
        $_.Name -ne "checksums.txt"
    } | Sort-Object Name | ForEach-Object {
        $hash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $($_.Name)"
    } | Set-Content -LiteralPath $checksumPath -Encoding ascii

    Write-Output "portable Windows artifact: $archivePath"
    Write-Output "portable longest path: $longestPath"
}
finally {
    if (Test-Path -LiteralPath $temporaryPath) {
        Remove-Item -LiteralPath $temporaryPath -Recurse -Force
    }
}

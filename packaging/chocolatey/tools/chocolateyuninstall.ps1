$ErrorActionPreference = 'Stop'

$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition

$packageArgs = @{
  packageName   = 'sth'
  zipFileName   = 'sth-windows-amd64.zip'
  unzipLocation = $toolsDir
}

Uninstall-ChocolateyZipPackage @packageArgs

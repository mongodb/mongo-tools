# Copyright (c) 2020-Present MongoDB Inc.
<#
.SYNOPSIS
    Builds an MSI for the MongoDB Tools product.
.DESCRIPTION
    .
#>
Param(
  [string]$VersionLabel
)

$ErrorActionPreference = 'Stop'

$ProjectName = "MongoDB Tools"
$sourceDir = pwd
$resourceDir = pwd
$binDir = pwd
$objDIr = ".\objs\"
$WixPath = "C:\wixtools\bin\"
$wixUiExt = "$WixPath\WixUIExtension.dll"

if (-not ($VersionLabel -match "(\d?\d?\d\.\d?\d?\d).*")) {
    throw "invalid version specified: $VersionLabel"
}
$version = $matches[1]

# upgrade code needs to change everytime we
# rev the minor version (1.0 -> 1.1). That way, we
# will allow multiple minor versions to be installed
# side-by-side.
if ([double]$version -gt 49.0) {
    throw "You must change the upgrade code for a minor revision.
Once that is done, change the version number above to
account for the next revision that will require being
upgradeable. Make sure to change both x64 and x86 upgradeCode"
}

$upgradeCode = "56c0fda6-289a-4fd0-a539-6711864146ba"
$Arch = "x64"

# compile wxs into .wixobjs
& $WixPath\candle.exe -wx `
    -dProductId="*" `
    -dPlatform="$Arch" `
    -dUpgradeCode="$upgradeCode" `
    -dVersion="$version" `
    -dVersionLabel="$VersionLabel" `
    -dProjectName="$ProjectName" `
    -dSourceDir="$sourceDir" `
    -dResourceDir="$resourceDir" `
    -dSslDir="$binDir" `
    -dBinaryDir="$binDir" `
    -dTargetDir="$objDir" `
    -dTargetExt=".msi" `
    -dTargetFileName="release" `
    -dOutDir="$objDir" `
    -dConfiguration="Release" `
    -arch "$Arch" `
    -out "$objDir" `
    -ext "$wixUiExt" `
    "$resourceDir\Product.wxs" `
    "$resourceDir\FeatureFragment.wxs" `
    "$resourceDir\BinaryFragment.wxs" `
    "$resourceDir\LicensingFragment.wxs" `
    "$resourceDir\UIFragment.wxs"

if(-not $?) {
    exit 1
}

$artifactsDir = pwd

# link wixobjs into an msi
& $WixPath\light.exe -wx `
    -cultures:en-us `
    -out "$artifactsDir\mongodb-tools-$VersionLabel-win-x86-64.msi" `
    -ext "$wixUiExt" `
    $objDir\Product.wixobj `
    $objDir\FeatureFragment.wixobj `
    $objDir\BinaryFragment.wixobj `
    $objDir\LicensingFragment.wixobj `
    $objDir\UIFragment.wixobj

trap {
  write-output $_
  exit 1
}

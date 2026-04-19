# This file is auto-updated by GoReleaser on each release.
# Do not edit manually — changes will be overwritten.
{ lib, stdenvNoCC, fetchurl }:
let
  version = "0.0.11";
  tarballs = {
    "aarch64-darwin" = fetchurl {
      url = "https://github.com/flaticols/gorefactor/releases/download/v${version}/gorefact_Darwin_arm64.tar.gz";
      hash = "sha256-EM7x2slKIM8Rv6giKiL000JCj4k67Qg7ajDSQj/p6FM="; # arm
    };
    "x86_64-darwin" = fetchurl {
      url = "https://github.com/flaticols/gorefactor/releases/download/v${version}/gorefact_Darwin_x86_64.tar.gz";
      hash = "sha256-cI3K9stl/X2HLBVtXLMzT97ArOs9T8MguYq3NgfDLKM="; # x86
    };
  };
in
stdenvNoCC.mkDerivation {
  pname = "gorefact";
  inherit version;

  src = tarballs.${stdenvNoCC.hostPlatform.system}
    or (throw "Unsupported system: ${stdenvNoCC.hostPlatform.system}. gorefact ships macOS binaries only.");

  unpackPhase = "tar xzf $src";

  installPhase = ''
    runHook preInstall
    install -Dm755 gorefact $out/bin/gorefact
    runHook postInstall
  '';

  meta = {
    description = "Go call-graph explorer and architectural dependency rule checker";
    homepage = "https://github.com/flaticols/gorefactor";
    license = lib.licenses.mit;
    mainProgram = "gorefact";
    platforms = [ "aarch64-darwin" "x86_64-darwin" ];
  };
}

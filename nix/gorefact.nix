# This file is auto-updated by GoReleaser on each release.
# Do not edit manually — changes will be overwritten.
{ lib, stdenvNoCC, fetchurl }:
let
  version = "0.0.14";
  tarballs = {
    "aarch64-darwin" = fetchurl {
      url = "https://github.com/flaticols/gorefactor/releases/download/v${version}/gorefact_Darwin_arm64.tar.gz";
      hash = "sha256-5CjlzGTH8NPMe4NqSHpQ/t59U96b3SlO0q2ycfQRBAg="; # arm
    };
    "x86_64-darwin" = fetchurl {
      url = "https://github.com/flaticols/gorefactor/releases/download/v${version}/gorefact_Darwin_x86_64.tar.gz";
      hash = "sha256-OgiWLwhP9c7Lk9Y/Fx/8Ap0TebwD49I9UDpE+PIPGsA="; # x86
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

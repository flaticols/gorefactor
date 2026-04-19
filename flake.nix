{
  description = "Go call-graph explorer and architectural dependency rule checker";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in {
      packages = forAllSystems (pkgs: {
        # Pre-built binary from the latest release (auto-updated by GoReleaser).
        default = pkgs.callPackage ./nix/gorefact.nix {};

        # Build from source — useful for development or patching.
        source = pkgs.buildGoModule rec {
          pname = "gorefact";
          version = "0.0.18";
          src = ./.;
          go = pkgs.go;
          ldflags = [ "-s" "-w" "-X main.Version=${version}" ];
          vendorHash = "sha256-cMZ0bNUDtTAnp2PdpdS+Ia53qm+SHe3AqMf/pH9gykU=";
          doCheck = false; # tests call packages.Load which requires network
          meta = with pkgs.lib; {
            description = "Go call-graph explorer and architectural dependency rule checker";
            homepage = "https://github.com/flaticols/gorefactor";
            license = licenses.mit;
            mainProgram = "gorefact";
            platforms = platforms.darwin;
          };
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [ go gopls gotools ];
        };
      });
    };
}

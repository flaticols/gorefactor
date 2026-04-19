{
  description = "Go call-graph explorer and architectural dependency rule checker";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in {
      packages = forAllSystems (pkgs: {
        default = pkgs.buildGoModule {
          pname = "gorefact";
          version = "0.0.7";
          src = ./.;
          go = pkgs.go;
          vendorHash = "sha256-cMZ0bNUDtTAnp2PdpdS+Ia53qm+SHe3AqMf/pH9gykU=";
          doCheck = false; # tests call packages.Load which requires network
          meta = with pkgs.lib; {
            description = "Inspect Go package dependencies, reference trees, and architectural rule violations";
            homepage = "https://github.com/flaticols/gorefactor";
            license = licenses.mit;
            mainProgram = "gorefact";
            platforms = platforms.unix;
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

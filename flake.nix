{
  description = "Bakery CLI — project scaffolding powered by composable pieces";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });

      # Build the bakery package for a given nixpkgs instantiation.
      bakeryFor = pkgs:
        pkgs.buildGoModule {
          pname = "bakery";
          version = "0.1.0";
          src = ./.;
          vendorHash = "sha256-ebBSFDf2cmdqI7kLnosU0Inn2RQzWTenxHqf5cQd5Is=";
          go = pkgs.go_1_27;
          ldflags = [ "-s" "-w" ];
          meta = with pkgs.lib; {
            description = "Project scaffolding CLI powered by composable pieces";
            homepage = "https://github.com/bakery-dev/bakery";
            license = licenses.mit;
            maintainers = [ ];
            mainProgram = "bakery";
          };
        };
    in
    {
      packages = forAllSystems (system:
        let pkgs = nixpkgsFor.${system};
        in {
          bakery = bakeryFor pkgs;
          default = self.packages.${system}.bakery;
        });

      overlays.default = final: _prev: {
        bakery = self.packages.${final.system}.bakery;
      };

      devShells = forAllSystems (system:
        let
          pkgs = nixpkgsFor.${system};
          rawVersion = builtins.readFile ./.golangci-version;
          golangciVersion = nixpkgs.lib.trim (nixpkgs.lib.removePrefix "v" rawVersion);
          golangci-lint-pinned = pkgs.golangci-lint.overrideAttrs (oldAttrs: {
            version = golangciVersion;
            __intentionallyOverridingVersion = true;
          });
        in {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
              go_1_27
              gopls
              gotools
              golangci-lint-pinned
              git
            ];

            shellHook = ''
              echo "Bakery CLI Dev Shell initialized. Go is available (golangci-lint ${golangciVersion})."
            '';
          };
        });
    };
}

{
  description = "Bakery CLI Development Environment";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }: 
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
      nixpkgsFor = forAllSystems (system: import nixpkgs { inherit system; });
    in {
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
              go
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

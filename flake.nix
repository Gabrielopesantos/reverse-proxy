{
  description = "A configurable HTTP reverse proxy with middleware support";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "reverse-proxy";
            version = "0.1.0";

            src = ./.;

            vendorHash = "sha256-mflBD79yQMdLG4aXyUnGWI0oLLWvikG2ffFvBIMtMYo=";

            meta = {
              description = "A configurable HTTP reverse proxy with middleware support";
              license = pkgs.lib.licenses.mit;
              mainProgram = "reverse-proxy";
            };
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools # goimports, godoc, etc.
            golangci-lint
          ];

          shellHook = ''
            echo "reverse-proxy dev shell (Go $(go version | awk '{print $3}'))"
          '';
        };
      }
    );
}

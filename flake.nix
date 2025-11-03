{
  description = "app-busca-search development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go toolchain
            go_1_24
            gopls
            gotools
            go-tools
            
            # Development tools
            just
            nodejs
            
            # Docker for containerization
            docker
            docker-compose
            
            # Git and version control
            git
            
            # JSON processing
            jq
            
            # Optional: useful development utilities
            curl
            wget
            htop
            tree
          ];

          shellHook = ''
            echo "🚀 Go development environment loaded!"
            echo "Go version: $(go version)"
            echo "Project: app-busca-search"
            echo ""
            echo "Available commands:"
            echo "  just run      - Run the application"
            echo "  just swagger  - Generate Swagger documentation"
            echo "  just start    - Generate docs and run"
            echo "  just build    - Build the application"
            echo "  just test     - Run tests"
            echo "  just tidy     - Tidy Go modules"
            echo ""
            
            # Set up Go environment
            export GOPATH="$PWD/.go"
            export GOCACHE="$PWD/.go/cache"
            export GOMODCACHE="$PWD/.go/mod"
            mkdir -p "$GOPATH" "$GOCACHE" "$GOMODCACHE"
            
            # Add Go bin to PATH for tools like swag
            export PATH="$GOPATH/bin:$PATH"
          '';

          # Environment variables
          CGO_ENABLED = "1";
        };
      });
}
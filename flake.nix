{
  description = "Multi-agent orchestration system for Claude Code with persistent work tracking";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    beads.url = "github:steveyegge/beads";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      beads,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        # Go 1.25.8: beads v0.60.0 deps (charm.land/huh/v2) require it, nixpkgs has 1.25.7
        # Remove when nixpkgs ships Go >= 1.25.8
        goOverlay = final: prev: {
          go_1_25 = prev.go_1_25.overrideAttrs {
            version = "1.25.8";
            src = prev.fetchurl {
              url = "https://go.dev/dl/go1.25.8.src.tar.gz";
              hash = "sha256-6YjUokRqx/4/baoImljpk2pSo4E1Wt7ByJgyMKjWxZ4=";
            };
          };
        };

        pkgs = import nixpkgs {
          inherit system;
          overlays = [ goOverlay ];
        };
        beadsPkg = beads.packages.${system}.default;
      in
      {
        packages = {
          gt = pkgs.buildGoModule {
            pname = "gt";
            version = "0.8.0";
            src = ./.;
            vendorHash = "sha256-XWv/slFm796AO928eqzVHms0uUX4ZMJk0I4mZz+kp54=";

            ldflags = [
              "-X github.com/steveyegge/gastown/internal/cmd.Build=nix"
              "-X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1"
            ];

            subPackages = [ "cmd/gt" ];

            meta = with pkgs.lib; {
              description = "Multi-agent orchestration system for Claude Code with persistent work tracking";
              homepage = "https://github.com/steveyegge/gastown";
              license = licenses.mit;
              mainProgram = "gt";
            };
          };
          default = self.packages.${system}.gt;
        };

        apps = {
          gt = flake-utils.lib.mkApp {
            drv = self.packages.${system}.gt;
          };
          default = self.apps.${system}.gt;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [
            beadsPkg
            pkgs.go_1_25
            pkgs.gopls
            pkgs.gotools
            pkgs.go-tools
          ];
        };
      }
    );
}

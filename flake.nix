{
  description = "lmao: review your unreviewed Lunch Money transactions in the terminal";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-darwin" "x86_64-darwin" "aarch64-linux" "x86_64-linux" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      # A release commit (made by the Release workflow, and only reachable
      # from its tag) has a VERSION file naming the tag, e.g. v0.2.0; main
      # doesn't, so builds from it are named by commit.
      release = if builtins.pathExists ./VERSION then nixpkgs.lib.trim (builtins.readFile ./VERSION) else "";
      version = if release != "" then release else self.shortRev or self.dirtyShortRev or "dev";
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.buildGoModule {
          pname = "lmao";
          version = nixpkgs.lib.removePrefix "v" version;
          src = self;

          # Update when go.mod/go.sum change, or when the code starts importing
          # a package from a dependency it didn't import before (only imported
          # packages are vendored): set to pkgs.lib.fakeHash, build, and copy
          # the hash from the error. Dependabot's PRs get it updated by
          # .github/workflows/dependabot-vendor-hash.yml.
          vendorHash = "sha256-2EW5WNvwlzc+2LvthcO+zMOfg9AJevo2/yr8ZGEYxVA=";

          env.CGO_ENABLED = 0;
          # Nix builds from a copy of the source without .git, so Go can't
          # record the version itself.
          ldflags = [ "-s" "-w" "-X github.com/adnathanail/lmao/cmd.stampedVersion=${version}" ];

          nativeBuildInputs = [ pkgs.installShellFiles ];

          postInstall = pkgs.lib.optionalString (pkgs.stdenv.buildPlatform.canExecute pkgs.stdenv.hostPlatform) ''
            installShellCompletion --cmd lmao \
              --bash <($out/bin/lmao completion bash) \
              --fish <($out/bin/lmao completion fish) \
              --zsh <($out/bin/lmao completion zsh)
          '';

          meta = {
            description = "Review your unreviewed Lunch Money transactions in the terminal";
            homepage = "https://github.com/adnathanail/lmao";
            mainProgram = "lmao";
          };
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gotools pkgs.golangci-lint pkgs.actionlint pkgs.prek ];
        };
      });

      # Home Manager module: installs lmao, optionally reading the access
      # token from a file.
      homeModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.programs.lmao;

          # lmao, wrapped to read LUNCHMONEY_TOKEN from tokenFile when it
          # starts (unless it's already set), so the token is only in lmao's
          # environment, not every shell's.
          wrapped = pkgs.symlinkJoin {
            name = "lmao-${cfg.package.version}";
            paths = [ cfg.package ];
            nativeBuildInputs = [ pkgs.makeWrapper ];
            postBuild = ''
              wrapProgram $out/bin/lmao --run ${lib.escapeShellArg ''
                if [ -z "''${LUNCHMONEY_TOKEN:-}" ]; then
                  LUNCHMONEY_TOKEN="$(cat ${lib.escapeShellArg cfg.tokenFile})" || exit 1
                  export LUNCHMONEY_TOKEN
                fi
              ''}
            '';
            inherit (cfg.package) meta;
          };
        in
        {
          options.programs.lmao = {
            enable = lib.mkEnableOption "lmao";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
              defaultText = lib.literalExpression "lmao.packages.\${system}.default";
              description = "The lmao package to install.";
            };

            tokenFile = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              default = null;
              example = "/run/agenix/lunchmoney-token";
              description = ''
                An absolute path to a file holding your Lunch Money access
                token, e.g. a secret agenix decrypts at activation. It's read
                each time lmao starts, unless LUNCHMONEY_TOKEN is already set.
                A string rather than a path, so the token isn't copied into
                the Nix store. Null means lmao uses LUNCHMONEY_TOKEN from the
                environment.
              '';
            };
          };

          config = lib.mkIf cfg.enable {
            assertions = [
              {
                assertion = cfg.tokenFile == null || lib.hasPrefix "/" cfg.tokenFile;
                message = "programs.lmao.tokenFile must be an absolute path.";
              }
            ];

            home.packages = [ (if cfg.tokenFile == null then cfg.package else wrapped) ];
          };
        };

      # The older name for homeModules, still what many configs use.
      homeManagerModules = self.homeModules;
    };
}

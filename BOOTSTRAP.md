# Bootstrap a machine

These steps take a machine from nothing to cloned sources and a written overlay configuration. Rendering commands arrive in later releases.

0. Prerequisites.
   - `git` on `PATH`. `overlay bootstrap` runs it to clone sources and to read the origin of an existing checkout.
   - SSH access to GitHub. The sources below are private `git@github.com:` URLs, and overlay never lets git prompt for credentials.
   - [mise](https://mise.jdx.dev) installed.

1. Install overlay.

   ```sh
   mise use -g github:jmcampanini/overlay
   ```

2. Bootstrap. Replace `dotfiles-xyz`, `~/dotfiles/xyz`, and `abc,xyz` with your sources and layers; each flag is one line so a source pair or layer list is easy to add or change.

   ```sh
   overlay bootstrap \
     --source-url git@github.com:jmcampanini/dotfiles-base.git \
     --source-path ~/dotfiles/base \
     --source-url git@github.com:jmcampanini/dotfiles-xyz.git \
     --source-path ~/dotfiles/xyz \
     --layers abc,xyz
   ```

`overlay bootstrap` refuses to run when the configuration file already exists, so this is a once-per-machine step; `overlay bootstrap --help` describes the full contract.

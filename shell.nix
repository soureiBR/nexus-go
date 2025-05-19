{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    gcc
    gnumake
    sqlite
    pkg-config
  ];
  
  shellHook = ''
    export CGO_ENABLED=1
  '';
}
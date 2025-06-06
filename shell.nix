{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    gcc
    gnumake
    sqlite
    pkg-config
    docker
    docker-compose
  ];
  
  shellHook = ''
    export CGO_ENABLED=1
  '';
}
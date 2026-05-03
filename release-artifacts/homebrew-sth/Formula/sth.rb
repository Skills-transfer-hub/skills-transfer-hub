# Placeholder Formula. Overwritten by GoReleaser-generated dist/sth.rb on each
# release (see release-artifacts/README.md in the source repo for sync steps).
class Sth < Formula
  desc "Skills Transfer Hub CLI — install reusable AI skills, agents and commands."
  homepage "https://github.com/Grandpied33/sth-releases"
  version "0.0.0"

  on_macos do
    on_arm do
      url "https://github.com/Grandpied33/sth-releases/releases/latest/download/sth-darwin-arm64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
    on_intel do
      url "https://github.com/Grandpied33/sth-releases/releases/latest/download/sth-darwin-amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/Grandpied33/sth-releases/releases/latest/download/sth-linux-amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
  end

  def install
    bin.install "sth"
  end

  test do
    assert_match "STH", shell_output("#{bin}/sth version")
  end
end

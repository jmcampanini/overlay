class Overlay < Formula
  desc "Merge layered JSON/TOML/YAML configuration files by profile"
  homepage "https://github.com/jmcampanini/overlay"
  license "MIT"
  head "https://github.com/jmcampanini/overlay.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = %W[
      -s -w
      -X github.com/jmcampanini/overlay/cmd.Version=#{version}
    ]
    system "go", "build", "-buildvcs=false", *std_go_args(output: bin/"overlay", ldflags:), "."
    generate_completions_from_executable(bin/"overlay", "completion")
  end

  test do
    assert_match "overlay version HEAD-", shell_output("#{bin}/overlay --version")
  end
end

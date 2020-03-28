class glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/489448124/artifacts/raw/archive/kubectl-glance-0.0.1.tar.gz?job=build-darwin"
    sha256 "fdb6d8ea9c301a119a1840c16fc0d4819710594ddcb66954274aa628b9eddeb4"
    version "0.0.1"
    
    def install
      bin.install "davidxarnold/glance"
    end

    test do
      kubectl-glance --help
    end
  
  end
  

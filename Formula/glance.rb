class glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/artifacts/master/raw/archive/0.0.1.tar.gz?job=build-darwin"
    sha256 "206478397c7dfd9db5d1ea79b34d2df15011bb2d58ba5c576d766871a5fd7547"
    version "0.0.1"
    
    def install
      bin.install "davidxarnold/glance"
    end

    test do
      kubectl-glance --help
    end
  
  end
  

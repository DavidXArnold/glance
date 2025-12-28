class Glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/499407379/artifacts/raw/archive/kubectl-glance-0.0.1.tar.gz?job=build-darwin"
    sha256 "b4ae0da1a8f6c732bb504d646a4d9856ec4a23cc9c4457a3f2fec8bd6ad98c5b"
    version "0.1.7"
    
    def install
      bin.install "kubectl-glance"
    end

    test do
      "kubectl-glance --help"
    end
  
  end
  

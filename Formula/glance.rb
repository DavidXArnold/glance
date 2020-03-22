class glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/artifacts/master/raw/archive/0.0.1.tar.gz?job=build-darwin"
    sha256 "f443438ad4d94977d47a6c5724d540b20be128c586b0fa224ed8f4b9731be4ab"
    version "0.0.1"
    
    def install
      bin.install "davidxarnold/glance"
    end

    test do
      kubectl-glance --help
    end
  
  end
  

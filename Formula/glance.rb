class glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/artifacts/master/raw/archive/0.0.1.tar.gz?job=build-darwin"
    sha256 ""
    version "0.0.1"
    
    bottle :unneeded
  
    def install
      bin.install "glance"
    end
  end
  

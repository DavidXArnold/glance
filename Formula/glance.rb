class Glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/499401216/artifacts/raw/archive/kubectl-glance-0.0.1.tar.gz?job=build-darwin"
    sha256 "6fe39ca45beb6334dcaed6c27e45d1d94162d4ab05bbb53b5f54386424cf6d5e"
    version "0.0.1"
    
    def install
      bin.install "kubectl-glance"
    end

    test do
      "kubectl-glance --help"
    end
  
  end
  

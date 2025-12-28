class Glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/499407379/artifacts/raw/archive/kubectl-glance-0.0.1.tar.gz?job=build-darwin"
    sha256 "6e6ac87023e027fb9355b7bf36a924d1483e3db5628562fdf75d1272f28a3d35"
    version "0.1.12"
    
    def install
      bin.install "kubectl-glance"
    end

    test do
      "kubectl-glance --help"
    end
  
  end
  

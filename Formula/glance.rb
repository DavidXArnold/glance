class Glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/499407379/artifacts/raw/archive/kubectl-glance-0.0.1.tar.gz?job=build-darwin"
    sha256 "06114aaad259bac8047325bc39853f912c4c69b87217aad2d8e25cb57cabc7e1"
    version "0.1.10"
    
    def install
      bin.install "kubectl-glance"
    end

    test do
      "kubectl-glance --help"
    end
  
  end
  

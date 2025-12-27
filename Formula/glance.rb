class Glance < Formula
    desc "A kubectl plugin to view cluster resource allocation and usage."
    homepage "https://github.com/davidxarnold/glance"
    url "https://gitlab.com/davidxarnold/glance/-/jobs/499407379/artifacts/raw/archive/kubectl-glance-0.0.1.tar.gz?job=build-darwin"
    sha256 "0152e96878935f8cca9fec9544e5f06dbd0ed06d48ec6f4ac4ef08d2c32a88d8"
    version "0.1.0"
    
    def install
      bin.install "kubectl-glance"
    end

    test do
      "kubectl-glance --help"
    end
  
  end
  

package collector

// RegisterNodeLocalCollectors registers all node-local collectors
func RegisterNodeLocalCollectors(registry Registry) error {
	collectors := []Collector{
		NewCPUCollector(),
		NewMemoryCollector(),
		NewDiskCollector(),
		NewNetworkCollector(),
		NewKubeletCollector(),
		NewContainerdCollector(),
		NewCNICollector(),
		NewKubeProxyCollector(),
		NewSystemLoadCollector(),
		NewFileDescriptorCollector(),
		NewKernelLogCollector(),
		NewNTPSyncCollector(),
	}

	for _, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			return err
		}
	}

	return nil
}

// RegisterClusterSupplementCollectors registers all cluster supplement collectors
func RegisterClusterSupplementCollectors(registry Registry) error {
	collectors := []Collector{
		&ProcessResourceCollector{},
		&KernelEventCollector{},
		&ConfigFileCollector{},
		&CertificateCollector{},
	}

	for _, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			return err
		}
	}
	return nil
}

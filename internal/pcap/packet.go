package pcap

import (
	"errors"
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

type triple struct {
	IP       string
	Port     uint16
	Protocol gopacket.LayerType
}

type quintuple struct {
	SrcIP    string
	SrcPort  uint16
	DstIP    string
	DstPort  uint16
	Protocol gopacket.LayerType
}

type devPacket struct {
	Packet gopacket.Packet
	Dev    *Device
	Handle *pcap.Handle
}

type devIndicator struct {
	Dev    *Device
	Handle *pcap.Handle
}

type natIndicator struct {
	SrcIP           string
	SrcPort         uint16
	EncappedSrcIP   string
	EncappedSrcPort uint16
	Dev             *Device
	Handle          *pcap.Handle
}

// sendTCPPacket implements a method sends a TCP packet
func sendTCPPacket(addr string, data []byte) error {
	// Create connection
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("send tcp packet: %w", err)
	}
	defer conn.Close()

	// Write data
	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("send tcp packet: %w", err)
	}
	return nil
}

// sendUDPPacket implements a method sends a UDP packet
func sendUDPPacket(addr string, data []byte) error {
	// Create connection
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return fmt.Errorf("send udp packet: %w", err)
	}
	defer conn.Close()

	// Write data
	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("send udp packet: %w", err)
	}
	return nil
}

type packetIndicator struct {
	NetworkLayer       gopacket.NetworkLayer
	NetworkLayerType   gopacket.LayerType
	SrcIP              net.IP
	DstIP              net.IP
	Id                 uint16
	TTL                uint8
	TransportLayer     gopacket.TransportLayer
	TransportLayerType gopacket.LayerType
	SrcPort            uint16
	DstPort            uint16
	Seq                uint32
	Ack                uint32
	SYN                bool
	ACK                bool
	ApplicationLayer   gopacket.ApplicationLayer
}

// SrcIPPort returns the source IP and port of the packet
func (indicator *packetIndicator) SrcIPPort() *IPPort {
	return &IPPort{
		IP:   indicator.SrcIP,
		Port: indicator.SrcPort,
	}
}

// DstIPPort returns the destination IP and port of the packet
func (indicator *packetIndicator) DstIPPort() *IPPort {
	return &IPPort{
		IP:   indicator.DstIP,
		Port: indicator.DstPort,
	}
}

// Payload returns the application layer in array of bytes
func (indicator *packetIndicator) Payload() []byte {
	if indicator.ApplicationLayer == nil {
		return nil
	}
	return indicator.ApplicationLayer.LayerContents()
}

func parsePacket(packet gopacket.Packet) (*packetIndicator, error) {
	var (
		networkLayer       gopacket.NetworkLayer
		networkLayerType   gopacket.LayerType
		srcIP              net.IP
		dstIP              net.IP
		id                 uint16
		ttl                uint8
		transportLayer     gopacket.TransportLayer
		transportLayerType gopacket.LayerType
		srcPort            uint16
		dstPort            uint16
		seq                uint32
		ack                uint32
		syn                bool
		bACK               bool
		applicationLayer   gopacket.ApplicationLayer
	)

	// Parse packet
	networkLayer = packet.NetworkLayer()
	if networkLayer == nil {
		return nil, fmt.Errorf("parse: %w", errors.New("missing network layer"))
	}
	networkLayerType = networkLayer.LayerType()
	transportLayer = packet.TransportLayer()
	if transportLayer == nil {
		return nil, fmt.Errorf("parse: %w", errors.New("missing transport layer"))
	}
	transportLayerType = transportLayer.LayerType()
	applicationLayer = packet.ApplicationLayer()

	// Parse network layer
	switch networkLayerType {
	case layers.LayerTypeIPv4:
		ipv4Layer := networkLayer.(*layers.IPv4)
		srcIP = ipv4Layer.SrcIP
		dstIP = ipv4Layer.DstIP
		id = ipv4Layer.Id
		ttl = ipv4Layer.TTL
	case layers.LayerTypeIPv6:
		ipv6Layer := networkLayer.(*layers.IPv6)
		srcIP = ipv6Layer.SrcIP
		dstIP = ipv6Layer.DstIP
	default:
		return nil, fmt.Errorf("parse: %w", fmt.Errorf("network layer type %s not support", networkLayerType))
	}

	// Parse transport layer
	switch transportLayerType {
	case layers.LayerTypeTCP:
		tcpLayer := transportLayer.(*layers.TCP)
		srcPort = uint16(tcpLayer.SrcPort)
		dstPort = uint16(tcpLayer.DstPort)
		seq = tcpLayer.Seq
		ack = tcpLayer.Ack
		syn = tcpLayer.SYN
		bACK = tcpLayer.ACK
	case layers.LayerTypeUDP:
		udpLayer := transportLayer.(*layers.UDP)
		srcPort = uint16(udpLayer.SrcPort)
		dstPort = uint16(udpLayer.DstPort)
	default:
		return nil, fmt.Errorf("parse: %w", fmt.Errorf("transport layer type %s not support", transportLayerType))
	}

	return &packetIndicator{
		NetworkLayer:       networkLayer,
		NetworkLayerType:   networkLayerType,
		SrcIP:              srcIP,
		DstIP:              dstIP,
		Id:                 id,
		TTL:                ttl,
		TransportLayer:     transportLayer,
		TransportLayerType: transportLayerType,
		SrcPort:            srcPort,
		DstPort:            dstPort,
		Seq:                seq,
		Ack:                ack,
		SYN:                syn,
		ACK:                bACK,
		ApplicationLayer:   applicationLayer,
	}, nil
}

func parseEncappedPacket(contents []byte) (*packetIndicator, error) {
	// Guess network layer type
	packet := gopacket.NewPacket(contents, layers.LayerTypeIPv4, gopacket.Default)
	networkLayer := packet.NetworkLayer()
	if networkLayer == nil {
		return nil, fmt.Errorf("parse encapped: %w", errors.New("missing network layer"))
	}
	if networkLayer.LayerType() != layers.LayerTypeIPv4 {
		return nil, fmt.Errorf("parse encapped: %w", errors.New("network layer type not support"))
	}
	ipVersion := networkLayer.(*layers.IPv4).Version
	switch ipVersion {
	case 4:
		break
	case 6:
		// Not IPv4, but IPv6
		encappedPacket := gopacket.NewPacket(contents, layers.LayerTypeIPv6, gopacket.Default)
		networkLayer = encappedPacket.NetworkLayer()
		if networkLayer == nil {
			return nil, fmt.Errorf("parse encapped: %w", errors.New("missing network layer"))
		}
		if networkLayer.LayerType() != layers.LayerTypeIPv6 {
			return nil, fmt.Errorf("parse encapped: %w", errors.New("network layer type not support"))
		}
	default:
		return nil, fmt.Errorf("parse encapped: %w", fmt.Errorf("ip version %d not support", ipVersion))
	}

	// Parse packet
	indicator, err := parsePacket(packet)
	if err != nil {
		return nil, fmt.Errorf("parse encapped: %w", err)
	}
	return indicator, nil
}

func parseRawPacket(contents []byte) (*gopacket.Packet, error) {
	// Guess link layer type
	packet := gopacket.NewPacket(contents, layers.LayerTypeLoopback, gopacket.Default)
	if len(packet.Layers()) < 0 {
		return nil, fmt.Errorf("parse raw: %w", errors.New("missing link layer"))
	}
	linkLayer := packet.Layers()[0]
	if linkLayer == nil {
		return nil, fmt.Errorf("parse raw: %w", errors.New("missing link layer"))
	}
	if linkLayer.LayerType() != layers.LayerTypeLoopback {
		// Not Loopback, then Ethernet
		packet = gopacket.NewPacket(contents, layers.LayerTypeEthernet, gopacket.Default)
		linkLayer := packet.LinkLayer()
		if linkLayer == nil {
			return nil, fmt.Errorf("parse raw: %w", errors.New("missing link layer"))
		}
		if linkLayer.LayerType() != layers.LayerTypeEthernet {
			return nil, fmt.Errorf("parse raw: %w", errors.New("link layer type not support"))
		}
	}

	return &packet, nil
}
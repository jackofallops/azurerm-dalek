package dalek

import (
	"github.com/jackofallops/azurerm-dalek/clients"
	"github.com/jackofallops/azurerm-dalek/dalek/options"
)

type Dalek struct {
	client *clients.AzureClient
	opts   options.Options
}

func NewDalek(client *clients.AzureClient, opts options.Options) Dalek {
	return Dalek{
		client: client,
		opts:   opts,
	}
}

// Package nswclient is the anti-corruption layer for the external NSW service.
//
// It is the single place that knows how to speak the NSW wire protocol:
// constructing callback URLs, building the "Style B" command/payload envelope,
// calling the NSW backend's storage endpoints, and fetching allowlisted
// consignment display names for Agency officers. Domain packages depend on
// small consumer-defined interfaces that this package satisfies, so NSW
// protocol details never leak into business logic.
//
// Nothing in this package imports the domain packages (application, storage);
// the dependency direction is domain -> nswclient -> pkg/httpclient -> NSW.
package nswclient

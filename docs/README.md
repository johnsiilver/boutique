# Boutique

## One line summary

Boutique is an immutable data store with subscriptions to field changes.

## The long summary

Boutique provides a data store for storing immutable data.  This allows data retrieved from the store to be used without providing synchronization even as changes are made to data in the store by other go-routines.

In addition, Boutique allows subscriptions to be registered for changes to a data field or any field changes.  Data is versioned, so you can compare the version number between the data retrieved and the last data pulled.

Finally, Boutique supports middleware for any change that is being committed to the store.



## Previous works

Boutique is based on the Redux library: http://redux.js.org




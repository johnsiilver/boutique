# Boutique

## One line summary

Boutique is an immutable data store with subscriptions to field changes.

## The long summary

Boutique provides a data store for storing immutable data.  This allows data retrieved from the store to be used without providing synchronization even as changes are made to data in the store by other go-routines.

In addition, Boutique allows subscriptions to be registered for changes to a data field or any field changes.  Data is versioned, so you can compare the version number between the data retrieved and the last data pulled.

Finally, Boutique supports middleware for any change that is being committed to the store.  This allows for sets of features, such as a storage record of changes to the store.

## Before we get started

### Go doesn't have immutable objects, does it?

Correct, Go doesn't have immutable objects.  It does contain immutable types, such as strings and constants.  However, immutability in this case is simply a contract to only change the data through the Boutique store.  All changes in the store must copy the data before committing the changes.

We will cover how this works later.

## What is the cost of using a generic immutable store?

There are three main drawbacks for using Boutique:

* Boutique writes are slower than a non generic implementation due to type assertion,  reflection and data copies
* In very certain circumstances, Boutique can have runtime errors due to using interface{}
* Storage updates are done via Actions, which adds some complexity

The first, running slower is because we must not only type assert at different points, but reflection is used to detect changes in the data fields that are in the data being stored.  This cost can be made up for by reads of data without synchronization and reduced complexity in the subscription model.

The second, runtime errors, happen when one of two events occur.  The type of data to be stored in Boutique is changed on a write.  The first data passed to the store is the only type that can be stored.  Any attempt to store a different type of data will be 

## Previous works

Boutique is based on the Redux library: [http://redux.js.org](http://redux.js.org)


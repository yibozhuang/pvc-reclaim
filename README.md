# pvc-reclaim

Controller for handling reclaiming released PersistentVolume.

The idea here is to have a CRD which is automatically
created upon PVC bound to PV.

Once PVC has been deleted, users have the option to
update this PVCReclaim resource in order to have
a new PVC created that is Bound back to a Released PV
to recover their previous PVC data.

Once the PV is actually deleted, the PVCReclaim will also
be deleted and at that point there is no way to recover.

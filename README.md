# podmove
move a pod which is controlled by a ReplicaSet or ReplicationController.

# Three steps
 1. Create a clone Pod of the original Pod
 
   The cloned pod **podA** has everything except the labels and podName of the original Pod.
   Since **podA** has no labels, no controller will try to adopt it.
   
 2. Delete the original Pod
 
   Once the original Pod get deleted, the controller will create another new Pod **podB**.
   
 3. Update the new Pod **podA** by adding the labels
 
   Once **podA** get the labels, the matched controller will try to [adopt it](https://github.com/kubernetes/kubernetes/blob/fa557ee7921fc8305d4978e66eb653c92ed1a7ce/pkg/controller/replicaset/replica_set.go#L333). After the adoption, the number of living pods
   for the controller will be one more than the specified replica number, so controller will select one pod to delete.
   In this case, both **podA** ad **podB** is running, but **podA** is older than **podB**, so **podB** will get deleted. 
 
 

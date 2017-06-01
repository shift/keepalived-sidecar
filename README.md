# keepalived-sidecar
A side car deplyed with any application who want a vip for HA

# How to use

* First, create a  ReplicationController (many replicas, using host network) with the keepalived-sidecar:

```
apiVersion: v1
kind: ReplicationController
metadata:
  namespace: "kube-system"
  name: "applitaion-test1"
  labels:
    keepalived: "application"
spec:
  replicas: 2
  selector:
    keepalived: "application"
  template:
   metadata:
    labels:
      keepalived: "application"
   spec:
    hostNetwork: true
    containers:
      - image: "keepalived-sidecar:v0.1.0"
        imagePullPolicy: "Always"
        name: "keepalived-sidecar"
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
          requests:
            cpu: 10m
            memory: 10Mi
        securityContext:
          privileged: true
        env:
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: SERVICE_NAME
            value: "application-service"
      - image: "any-application:v1.0.0"
        imagePullPolicy: "Always"
        name: "any-application"
        resources:
          limits:
            cpu: 200m
            memory: 200Mi
          requests:
            cpu: 200m
            memory: 200Mi
```

* Then create a Service named "application-service" pointing to the RC and assign a vip to the service using annotation:
```
apiVersion: v1
kind: Service
metadata:
  name: application-service
  namespace: kube-system
  annotations:
    "keepalived.k8s.io/vip": "192.168.10.1"
    "keepalived.k8s.io/vrid": "50"
spec:
  type: ClusterIP
  selector:
    keepalived: "application"
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 80
```

* keepalived-sidecar container will watch the specified Service and update keepalived's config.
* User could access in-cluster service by the keepalived VIP.

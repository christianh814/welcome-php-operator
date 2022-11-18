# welcome-php-operator
An Operator to deploy the Welcome PHP Application

# Deploy

First, make sure you've got the updated types and such

```
make generate
```

Build (i.e. compile) the Go Operator in a container
```
make docker-build docker-push IMG=quay.io/christianh814/welcomephp-operator:v0.0.1
```

Generate the manifests

```
make manifests
```

Collect everything in the `deploy` directory

```
export IMG=quay.io/christianh814/welcomephp-operator:v0.0.1
cd config/manager/
kustomize edit set image controller=${IMG}
cd ../../
kustomize build config/default > deploy/welcomephp-operator.yaml
```

Create the CR yaml for your Operator (example is in the `deploy` directory)

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: test
---
apiVersion: capp.chernand.io/v1alpha1
kind: WelcomePhp
metadata:
  name: welcomephp-sample
  namespace: test
spec:
  size: 3
```

Apply the Operator

```
kubectl apply -f deploy/welcomephp-operator.yaml
```

Apply the CR into the `test` namespace

```
kubectl apply -f welcome-php.yaml 
```

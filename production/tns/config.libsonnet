local tk = import 'tk';
{
  jaeger: {
    host: 'jaeger.jaeger.svc.cluster.local',
    tags: 'cluster=tns,namespace=tns',
    sampler_type: 'const',
    sampler_param: '1',
  },

  images+:: {
    tns_app: 'grafana/tns-app:latest',
    loadgen: 'grafana/tns-loadgen:latest',
    db: 'grafana/tns-db:latest',
  },
  
  namespace: tk.env.spec.namespace,
}

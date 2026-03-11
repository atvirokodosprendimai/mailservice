import os

from diagrams import Cluster, Diagram, Edge
from diagrams.generic.compute import Rack
from diagrams.generic.network import Firewall
from diagrams.generic.place import Datacenter
from diagrams.generic.storage import Storage
from diagrams.onprem.ci import GithubActions


graph_attr = {
    "fontsize": "14",
    "bgcolor": "#ffffff",
    "pad": "0.5",
    "nodesep": "0.7",
    "ranksep": "1.0",
    "splines": "spline",
}


with Diagram(
    "mailservice-deployment-view",
    filename=os.getenv("DIAGRAM_NAME", "deployment-view"),
    outformat=os.getenv("DIAGRAM_OUTFORMAT", "png"),
    show=False,
    direction="TB",
    graph_attr=graph_attr,
):
    github = GithubActions("GitHub Actions")
    cloudflare = Firewall("Cloudflare Tunnel")
    polar = Rack("Polar")

    with Cluster("Hetzner Cloud VM"):
        api = Rack("mailservice-api\nsystemd")
        postfix = Rack("postfix\ninbound SMTP")
        dovecot = Rack("dovecot2\nIMAP/LMTP")
        cloudflared = Rack("mailservice-cloudflared")
        sqlite = Storage("SQLite state")

    with Cluster("NixOS deployment"):
        repo_sync = Datacenter("git sync")
        rebuild = Datacenter("nixos-rebuild switch")
        health = Datacenter("host health check")

    github >> Edge(label="sync repo") >> repo_sync >> rebuild >> health
    github >> Edge(label="workflow + infra apply") >> api

    cloudflare >> Edge(label="public ingress") >> cloudflared >> api
    api >> Edge(label="payment session lookup") >> polar

    api >> Edge(label="provision mailbox") >> postfix
    api >> Edge(label="provision mailbox") >> dovecot
    api >> sqlite
    postfix >> sqlite
    dovecot >> sqlite

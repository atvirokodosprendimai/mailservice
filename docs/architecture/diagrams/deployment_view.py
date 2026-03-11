# pyright: reportMissingImports=false

import os

from diagrams import Cluster, Diagram, Edge  # type: ignore
from diagrams.generic.compute import Rack  # type: ignore
from diagrams.generic.network import Firewall  # type: ignore
from diagrams.generic.place import Datacenter  # type: ignore
from diagrams.generic.storage import Storage  # type: ignore
from diagrams.onprem.ci import GithubActions  # type: ignore


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
    cloudflare = Firewall("Cloudflare Edge/Tunnel")
    polar = Rack("Polar")
    turso = Storage("Turso (optional app DB mode)")

    with Cluster("Hetzner Cloud VM"):
        api = Rack("mailservice-api\nsystemd")
        postfix = Rack("postfix\ninbound SMTP")
        dovecot = Rack("dovecot2\nIMAP/LMTP")
        cloudflared = Rack("mailservice-cloudflared")
        sqlite = Storage("SQLite state")

    with Cluster("NixOS deploy sequence"):
        repo_sync = Datacenter("git sync")
        rebuild = Datacenter("nixos-rebuild switch")
        health = Datacenter("host-local health check")

    github >> Edge(label="deploy app revision") >> repo_sync >> rebuild >> health

    cloudflare >> Edge(label="public ingress") >> cloudflared >> api
    api >> Edge(label="payment session lookup") >> polar
    api >> Edge(label="app data when DB_MODE=turso") >> turso

    api >> Edge(label="provision mailbox") >> postfix
    api >> Edge(label="provision mailbox") >> dovecot
    api >> Edge(label="local DB mode or runtime tables") >> sqlite
    postfix >> sqlite
    dovecot >> sqlite

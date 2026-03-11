# pyright: reportMissingImports=false

import os

from diagrams import Cluster, Diagram, Edge  # type: ignore
from diagrams.generic.compute import Rack  # type: ignore
from diagrams.generic.database import SQL  # type: ignore
from diagrams.generic.network import Router  # type: ignore
from diagrams.onprem.client import User  # type: ignore


graph_attr = {
    "fontsize": "14",
    "bgcolor": "#ffffff",
    "pad": "0.5",
    "nodesep": "0.7",
    "ranksep": "1.0",
    "splines": "spline",
    "concentrate": "true",
}


with Diagram(
    "mailservice-container-view",
    filename=os.getenv("DIAGRAM_NAME", "container-view"),
    outformat=os.getenv("DIAGRAM_OUTFORMAT", "png"),
    show=False,
    direction="TB",
    graph_attr=graph_attr,
):
    client = User("Agent / API client")
    http_api = Router("HTTP API\ninternal/adapters/httpapi")

    with Cluster("Core"):
        core_services = Rack("Core services\ninternal/core/service")
        domain_ports = Rack("Domain + ports\ninternal/domain + internal/core/ports")

    with Cluster("Adapters"):
        repositories = Rack("Repositories\ninternal/adapters/repository")
        identity = Rack("Identity\ninternal/adapters/identity/edproof")
        payments = Rack("Payments\ninternal/adapters/payment")
        notify = Rack("Notifier\ninternal/adapters/notify")
        runtime_provisioner = Rack(
            "Mail runtime provisioner\ninternal/adapters/repository"
        )
        imap = Rack("IMAP reader\ninternal/adapters/imap")
        token = Rack("Token generator\ninternal/adapters/token")

    app_db = SQL("App DB\nSQLite or Turso")
    runtime_db = SQL("Local SQLite\nmail runtime tables")

    client >> Edge(label="HTTP JSON") >> http_api
    http_api >> core_services
    core_services >> domain_ports

    core_services >> Edge(label="CRUD") >> repositories
    repositories >> app_db

    core_services >> Edge(label="verify key proof") >> identity
    core_services >> Edge(label="create + verify session") >> payments
    core_services >> Edge(label="send email") >> notify
    (
        core_services
        >> Edge(label="provision mailbox records")
        >> runtime_provisioner
        >> runtime_db
    )
    core_services >> Edge(label="list/read messages") >> imap
    core_services >> Edge(label="mint tokens") >> token

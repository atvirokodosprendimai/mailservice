# pyright: reportMissingImports=false

import os

from diagrams import Cluster, Diagram, Edge  # type: ignore
from diagrams.generic.compute import Rack  # type: ignore
from diagrams.generic.device import Mobile  # type: ignore
from diagrams.generic.network import Firewall  # type: ignore
from diagrams.generic.place import Datacenter  # type: ignore
from diagrams.generic.storage import Storage  # type: ignore


graph_attr = {
    "fontsize": "14",
    "bgcolor": "#ffffff",
    "pad": "0.5",
    "nodesep": "0.7",
    "ranksep": "0.9",
    "splines": "spline",
}


with Diagram(
    "mailservice-system-context",
    filename=os.getenv("DIAGRAM_NAME", "system-context"),
    outformat=os.getenv("DIAGRAM_OUTFORMAT", "png"),
    show=False,
    direction="TB",
    graph_attr=graph_attr,
):
    agent = Mobile("Agent")
    billing_inbox = Storage("Billing inbox")
    operator = Rack("Operator")

    with Cluster("Product boundary: mailservice"):
        api = Datacenter("Mailservice API")

    with Cluster("External systems"):
        cloudflare = Firewall("Cloudflare Tunnel")
        polar = Rack("Polar")
        stripe = Rack("Stripe (legacy fallback)")
        notifier = Rack("Unsend / Resend /\nSendGrid / Mailgun")
        mail_runtime = Datacenter("Mail runtime\nPostfix + Dovecot")

    agent >> Edge(label="claim + resolve API calls") >> cloudflare >> api
    api >> Edge(label="payment link") >> billing_inbox
    polar >> Edge(label="hosted checkout") >> agent
    operator >> Edge(label="deploy + configure") >> api

    api >> Edge(label="checkout") >> polar
    api >> Edge(label="legacy fallback") >> stripe
    api >> Edge(label="send notifications") >> notifier
    api >> Edge(label="provision + IMAP read") >> mail_runtime
    cloudflare >> Edge(label="public ingress") >> api

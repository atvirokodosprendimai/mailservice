import os

from diagrams import Cluster, Diagram, Edge
from diagrams.generic.compute import Rack
from diagrams.generic.device import Mobile
from diagrams.generic.network import Firewall
from diagrams.generic.place import Datacenter
from diagrams.generic.storage import Storage


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

    with Cluster("mailservice"):
        api = Datacenter("Mailservice API")

    with Cluster("External systems"):
        polar = Rack("Polar")
        stripe = Rack("Stripe (legacy)")
        notifier = Rack("Resend / SendGrid / Unsend")
        mail_runtime = Datacenter("Mail runtime\nPostfix + Dovecot")
        cloudflare = Firewall("Cloudflare Tunnel")

    agent >> Edge(label="claim + resolve") >> api
    api >> Edge(label="payment link") >> billing_inbox
    operator >> Edge(label="deploy + configure") >> api

    api >> Edge(label="checkout") >> polar
    api >> Edge(label="legacy fallback") >> stripe
    api >> Edge(label="send notifications") >> notifier
    api >> Edge(label="provision + IMAP read") >> mail_runtime
    cloudflare >> Edge(label="public ingress") >> api

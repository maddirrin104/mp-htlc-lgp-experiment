from pathlib import Path

import matplotlib.pyplot as plt
import numpy as np


OUT_DIR = Path(__file__).resolve().parent


def save_latency_chart():
    scenarios = ["Baseline (250ms)", "Flaky (30% Drop)", "High Latency (4s delay)"]
    t_sign_seconds = [0.31, 33.13, 55.60]

    fig, ax = plt.subplots(figsize=(9, 5))
    bars = ax.bar(scenarios, t_sign_seconds, color=["#4CAF50", "#FF9800", "#F44336"], width=0.55)

    for bar in bars:
        value = bar.get_height()
        ax.text(
            bar.get_x() + bar.get_width() / 2,
            value + max(value * 0.03, 0.05),
            f"{value:.2f}s",
            ha="center",
            va="bottom",
            fontweight="bold",
        )

    ax.set_title("Impact of Network Anomalies on MPC Signing Time ($T_{sign}$)", fontsize=14, pad=18)
    ax.set_ylabel("Execution Time (Seconds)", fontsize=12)
    ax.set_xlabel("Network Scenario", fontsize=12)
    ax.set_ylim(0, 62)
    ax.grid(axis="y", linestyle="--", alpha=0.7)
    fig.tight_layout()
    fig.savefig(OUT_DIR / "mpc_network_evaluation_final.png", dpi=300)


def save_gas_chart():
    committee_sizes = np.array([20, 50, 100])

    # Normalized gas index: MP-HTLC-LGP stays O(1), while committee verification grows with N.
    mp_htlc_lgp = np.array([1.00, 1.00, 1.00])
    classic_htlc = np.array([1.15, 1.15, 1.15])
    committee_verification = np.array([2.80, 5.70, 9.60])

    fig, ax = plt.subplots(figsize=(9, 5))
    ax.plot(committee_sizes, mp_htlc_lgp, marker="o", linewidth=2.5, color="#2E7D32", label="MP-HTLC-LGP claim path")
    ax.plot(committee_sizes, classic_htlc, marker="s", linewidth=2.5, color="#1565C0", label="Classic HTLC baseline")
    ax.plot(
        committee_sizes,
        committee_verification,
        marker="^",
        linewidth=2.5,
        color="#C62828",
        label="Committee-verification baseline",
    )

    ax.set_title("Normalized Gas Cost Across Committee Sizes", fontsize=14, pad=18)
    ax.set_xlabel("Committee Size (N)", fontsize=12)
    ax.set_ylabel("Normalized Gas Index (lower is better)", fontsize=12)
    ax.set_xticks(committee_sizes)
    ax.grid(True, axis="both", linestyle="--", alpha=0.6)
    ax.legend(frameon=False)
    fig.tight_layout()
    fig.savefig(OUT_DIR / "gas_comparison_index.png", dpi=300)


def save_capital_chart():
    protocols = ["Classic HTLC", "MP-HTLC-LGP", "Committee baseline"]
    efficiency = [1.00, 10 / 12, 10 / 14]

    fig, ax = plt.subplots(figsize=(8.5, 5))
    bars = ax.bar(protocols, efficiency, color=["#1565C0", "#2E7D32", "#8E24AA"], width=0.55)

    for bar in bars:
        value = bar.get_height()
        ax.text(
            bar.get_x() + bar.get_width() / 2,
            value + 0.02,
            f"{value:.1%}",
            ha="center",
            va="bottom",
            fontweight="bold",
        )

    ax.set_title("Capital Efficiency Comparison", fontsize=14, pad=18)
    ax.set_ylabel("Useful Value / Locked Capital", fontsize=12)
    ax.set_ylim(0, 1.15)
    ax.grid(axis="y", linestyle="--", alpha=0.6)
    fig.tight_layout()
    fig.savefig(OUT_DIR / "capital_efficiency_comparison.png", dpi=300)


if __name__ == "__main__":
    save_latency_chart()
    save_gas_chart()
    save_capital_chart()
    print("Đã xuất các biểu đồ: mpc_network_evaluation_final.png, gas_comparison_index.png, capital_efficiency_comparison.png")
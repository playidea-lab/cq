#!/usr/bin/env python3
"""Hosted worker unit economics calculator."""

from __future__ import annotations

import argparse


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Calculate hosted worker unit cost and suggested price."
    )
    p.add_argument("--in-tokens", type=float, required=True, help="Input tokens per task")
    p.add_argument("--out-tokens", type=float, required=True, help="Output tokens per task")
    p.add_argument(
        "--in-rate",
        type=float,
        required=True,
        help="Input token price in USD per 1M tokens",
    )
    p.add_argument(
        "--out-rate",
        type=float,
        required=True,
        help="Output token price in USD per 1M tokens",
    )
    p.add_argument(
        "--runtime-sec",
        type=float,
        default=0.0,
        help="Average runtime seconds per task",
    )
    p.add_argument(
        "--cpu-usd-per-hour",
        type=float,
        default=0.0,
        help="Compute cost in USD per hour",
    )
    p.add_argument(
        "--fixed-overhead",
        type=float,
        default=0.0,
        help="Fixed infra/ops overhead USD per task",
    )
    p.add_argument(
        "--margin",
        type=float,
        default=0.35,
        help="Target gross margin ratio (0.0~0.95)",
    )
    p.add_argument(
        "--tasks-per-month",
        type=float,
        default=0.0,
        help="Optional monthly task volume for projection",
    )
    return p.parse_args()


def main() -> int:
    args = parse_args()
    if not (0.0 <= args.margin < 0.95):
        raise SystemExit("margin must be in [0.0, 0.95)")

    model_cost = (
        (args.in_tokens * args.in_rate) + (args.out_tokens * args.out_rate)
    ) / 1_000_000.0
    infra_cost = (args.runtime_sec * args.cpu_usd_per_hour / 3600.0) + args.fixed_overhead
    unit_cost = model_cost + infra_cost
    suggested_price = unit_cost / (1.0 - args.margin)
    gross_profit = suggested_price - unit_cost

    print(f"model_cost_usd={model_cost:.6f}")
    print(f"infra_cost_usd={infra_cost:.6f}")
    print(f"unit_cost_usd={unit_cost:.6f}")
    print(f"suggested_price_usd={suggested_price:.6f}")
    print(f"gross_profit_usd={gross_profit:.6f}")
    print(f"margin={args.margin:.2%}")

    if args.tasks_per_month > 0:
        monthly_cost = unit_cost * args.tasks_per_month
        monthly_revenue = suggested_price * args.tasks_per_month
        monthly_profit = monthly_revenue - monthly_cost
        print(f"monthly_tasks={args.tasks_per_month:.0f}")
        print(f"monthly_cost_usd={monthly_cost:.2f}")
        print(f"monthly_revenue_usd={monthly_revenue:.2f}")
        print(f"monthly_profit_usd={monthly_profit:.2f}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

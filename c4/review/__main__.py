"""
CLI entry point for auto_review.
"""

import os
from pathlib import Path

import typer

app = typer.Typer(
    name="auto_review",
    help="Automated academic paper review system using Claude API",
)


@app.command()
def review(
    pdf_path: str = typer.Argument(..., help="Path to the PDF file to review"),
    output_path: str = typer.Option("review.md", help="Output path for review document"),
):
    """
    Review an academic paper PDF and generate a structured review document.
    """
    typer.echo(f"Reviewing paper: {pdf_path}")
    typer.echo(f"Output will be saved to: {output_path}")
    typer.echo("Review functionality not yet implemented.")


@app.command()
def bootstrap_profile(
    review_dir: str = typer.Option("review/", help="Directory containing existing review files"),
    output_path: str = typer.Option(".auto_review/profile.yaml", help="Output path for profile"),
):
    """
    Bootstrap a reviewer profile from existing review files.

    Analyzes existing review.md files to extract reviewer patterns and preferences.
    """
    from c4.review.profile import ReviewProfile

    review_path = Path(review_dir)
    output_file = Path(output_path)

    if not review_path.exists():
        typer.secho(f"Error: Review directory not found: {review_path}", fg=typer.colors.RED, err=True)
        raise typer.Exit(code=1)

    # Get API key
    api_key = os.getenv("ANTHROPIC_API_KEY")
    if not api_key:
        typer.secho("Error: ANTHROPIC_API_KEY environment variable not set", fg=typer.colors.RED, err=True)
        raise typer.Exit(code=1)

    typer.echo(f"Bootstrapping profile from: {review_path}")
    typer.echo("Analyzing existing review files...")

    try:
        profile_mgr = ReviewProfile(api_key=api_key)
        profile = profile_mgr.bootstrap_from_existing(review_path)

        # Save the profile
        profile_mgr.save_profile(profile, output_file)

        typer.secho(f"✓ Profile saved to: {output_file}", fg=typer.colors.GREEN)
        typer.echo("\nExtracted patterns:")
        typer.echo(f"  - Review style: {profile.review_style}")
        typer.echo(f"  - Expertise areas: {', '.join(profile.expertise_areas) if profile.expertise_areas else 'None'}")
        typer.echo(f"  - Review points: {len(profile.review_points)} patterns identified")

    except Exception as e:
        typer.secho(f"Error: {e}", fg=typer.colors.RED, err=True)
        raise typer.Exit(code=1)


if __name__ == "__main__":
    app()

import sys
from setuptools import setup
import os

if sys.version_info < (3, 6):
    sys.exit("Please install with Python >= 3.6")

with open(os.path.join(os.path.dirname(__file__), "README.md"), encoding="utf-8") as f:
    readme = f.read()


setup(
    name="chkbit",
    version="2.0.2",
    url="https://github.com/laktak/chkbit-py",
    author="Christian Zangl",
    author_email="laktak@cdak.net",
    description="chkbit is a lightweight bitrot detection tool.",
    long_description=readme,
    long_description_content_type="text/markdown",
    entry_points={"console_scripts": ["chkbit = chkbit.main:main"]},
    packages=["chkbit"],
    install_requires=[],
    python_requires=">=3.6.0",
)

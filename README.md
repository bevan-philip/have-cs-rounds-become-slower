# Has CS become slower?
Project to analyse CS:GO demos to understand how the game has evolved over time

[Read the blog post that this project became.](https://bphilip.uk/2023/04/26/have-cs-rounds-gotten-slower)

## Structure
- main.go 
    - Ingestion engine. Creates SQLite3 rows from CS:GO demos, breaking down the outcome of every round played.
- analysis.ipynb
    - Jupyter notebook that analysed the outputted data.
    - This notebook was designed for information analysis to feed into a blog post, rather than being a published artifact itself.
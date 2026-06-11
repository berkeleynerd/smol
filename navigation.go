package main

import "fmt"

type PageNavigation struct {
	HasLinks bool
	Home     *PageNavigationLink
	Up       *PageNavigationLink
	Previous *PageNavigationLink
	Next     *PageNavigationLink
	Related  []PageNavigationLink
	Top      PageNavigationLink
}

type PageNavigationLink struct {
	Rel   string
	Href  string
	Label string
}

func resolvePageNavigations(pages, posts []Page, outputMode string) ([]Page, []Page, error) {
	targets := make(map[string]Page, len(pages))
	for _, page := range pages {
		targets[page.Slug] = page
	}
	for i := range pages {
		if pages[i].Draft {
			continue
		}
		nav, err := resolvePageNavigation(pages[i], targets, outputMode)
		if err != nil {
			return nil, nil, err
		}
		pages[i].Navigation = nav
	}
	for i := range posts {
		if posts[i].Draft {
			continue
		}
		nav, err := resolvePageNavigation(posts[i], targets, outputMode)
		if err != nil {
			return nil, nil, err
		}
		posts[i].Navigation = nav
	}
	return pages, posts, nil
}

func resolvePageNavigation(source Page, targets map[string]Page, outputMode string) (PageNavigation, error) {
	if !hasPageLinks(source.Links) {
		return PageNavigation{}, nil
	}
	nav := PageNavigation{
		HasLinks: true,
		Top: PageNavigationLink{
			Rel:   "top",
			Href:  "#top",
			Label: "Top",
		},
	}
	if home, ok := targets["index"]; ok && !home.Draft && source.Slug != "index" {
		link := navigationLink(source, home, outputMode, "home", "Home")
		nav.Home = &link
	}
	var err error
	nav.Up, err = resolveSingleNavigationLink(source, targets, outputMode, "up", source.Links.Up, "Up")
	if err != nil {
		return PageNavigation{}, err
	}
	if nav.Home != nil && source.Links.Up == "index" {
		nav.Up = nil
	}
	nav.Previous, err = resolveSingleNavigationLink(source, targets, outputMode, "previous", source.Links.Previous, "Previous")
	if err != nil {
		return PageNavigation{}, err
	}
	nav.Next, err = resolveSingleNavigationLink(source, targets, outputMode, "next", source.Links.Next, "Next")
	if err != nil {
		return PageNavigation{}, err
	}
	seenRelated := map[string]bool{}
	for _, slug := range source.Links.Related {
		if seenRelated[slug] {
			return PageNavigation{}, fmt.Errorf("page %q has duplicate related link target: %s", source.Slug, slug)
		}
		seenRelated[slug] = true
		link, err := resolveRequiredNavigationLink(source, targets, outputMode, "related", slug, "Related")
		if err != nil {
			return PageNavigation{}, err
		}
		nav.Related = append(nav.Related, link)
	}
	return nav, nil
}

func hasPageLinks(links PageLinks) bool {
	return links.Up != "" || links.Previous != "" || links.Next != "" || len(links.Related) > 0
}

func resolveSingleNavigationLink(source Page, targets map[string]Page, outputMode, rel, slug, labelPrefix string) (*PageNavigationLink, error) {
	if slug == "" {
		return nil, nil
	}
	link, err := resolveRequiredNavigationLink(source, targets, outputMode, rel, slug, labelPrefix)
	if err != nil {
		return nil, err
	}
	return &link, nil
}

func resolveRequiredNavigationLink(source Page, targets map[string]Page, outputMode, rel, slug, labelPrefix string) (PageNavigationLink, error) {
	target, ok := targets[slug]
	if !ok {
		return PageNavigationLink{}, fmt.Errorf("page %q links.%s has unknown target slug: %s", source.Slug, rel, slug)
	}
	if target.Draft {
		return PageNavigationLink{}, fmt.Errorf("page %q links.%s targets draft page: %s", source.Slug, rel, slug)
	}
	if target.Slug == source.Slug {
		return PageNavigationLink{}, fmt.Errorf("page %q links.%s cannot target itself", source.Slug, rel)
	}
	return navigationLink(source, target, outputMode, rel, labelPrefix+": "+navigationLabelName(target)), nil
}

func navigationLabelName(target Page) string {
	if target.NavLabel != "" {
		return target.NavLabel
	}
	return target.Title
}

func navigationLink(source, target Page, outputMode, rel, label string) PageNavigationLink {
	return PageNavigationLink{
		Rel:   rel,
		Href:  relativeHref(source.URL, target.URL, outputMode),
		Label: label,
	}
}

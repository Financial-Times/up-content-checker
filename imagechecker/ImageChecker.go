package imagechecker

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/Financial-Times/up-content-checker/util"
	xmlpath "gopkg.in/xmlpath.v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

type (
	checker struct {
		client *http.Client
	}

	Identifier struct {
		Authority       string `json:authority`
		IdentifierValue string `json:identifierValue`
	}

	MainImage struct {
		Id string `json:id`
	}

	Member struct {
		Id string `json:id`
	}

	Comments struct {
		Enabled bool `json:enabled`
	}

	Content struct {
		Id          string `json:id`
		ContentType string `json:type`
		BodyXML     string `json:bodyXML`
		Title       string `json:title`
		//		Byline           string       `json:byline`
		//		PublishedDate    string       `json:publishedDate`
		Identifiers []Identifier `json:identifiers`
		//		RequestUrl       string       `json:requestUrl`
		//		Brands           []string     `json:brands`
		MainImage MainImage `json:mainImage`
		BinaryUrl string    `json:binaryUrl`
		Members   []Member  `json:members`
		//		Comments         Comments     `json:comments`
		PublishReference string `json:publishReference`
		LastModified     string `json:lastModified`
	}
)

var (
	ErrCouldNotFetchContent = errors.New("Could not fetch content")
	contentUrl              string
	xpath                   = xmlpath.MustCompile("//ft-content[@type=\"http://www.ft.com/ontology/content/ImageSet\"]/@url")
)

func init() {
	flag.StringVar(&contentUrl, "content", "http://api.ft.com/content", "Content read endpoint URL")
}

func NewChecker(client *http.Client) util.Checker {
	c := &checker{client}

	return c
}

func (c checker) Check(uuid string) ([][]string, error) {
	//	log.Printf("Check UUID: %s", uuid)

	var (
		result [][]string
	)

	content, err := c.fetchContent(uuid)
	if err != nil {
		log.Printf("Unable to fetch content: %s", err.Error())

		return append(result, []string{uuid, err.Error()}), err
	}

	imageSets, err := listImageSets(content)
	if err != nil {
		log.Printf("Unable to parse document: %s", err.Error())
		return append(result, []string{uuid, err.Error()}), err
	}

	//	log.Printf("UUID: %s image sets: %s", uuid, imageSets)

	for _, imageSetUuid := range imageSets {
		imageSet, err := c.fetchContent(imageSetUuid)
		if err != nil {
			result = append(result, []string{uuid, imageSetUuid})
			continue
		}

		for _, imageSetMember := range imageSet.Members {
			imageModelUuid, found := util.ExtractUuid(imageSetMember.Id)
			if found {
				row := c.checkImageModel(uuid, imageSetUuid, imageModelUuid)
				if len(row) > 0 {
					result = append(result, row)
				}
			}
		}
	}

	return result, nil
}

func (c checker) fetchContent(uuid string) (*Content, error) {
	var content Content

	url := fmt.Sprintf("%s/%s", contentUrl, uuid)

	req, _ := http.NewRequest("GET", url, nil)
	util.AddBasicAuthentication(req)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("Unable to fetch content: %s: %s", url, err.Error())
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		//		log.Printf("Unexpected HTTP response %d", resp.StatusCode)
		return nil, ErrCouldNotFetchContent
	}

	err = json.NewDecoder(resp.Body).Decode(&content)
	io.Copy(ioutil.Discard, resp.Body) // consume everything
	if err != nil {
		log.Printf("Unable to deserialize JSON: %s", err)
		return nil, err
	}

	return &content, nil
}

func listImageSets(content *Content) ([]string, error) {
	imageSets := make(map[string]string)

	mainImage := content.MainImage
	if mainImageUuid, hasMainImage := util.ExtractUuid(mainImage.Id); hasMainImage {
		imageSets[mainImageUuid] = mainImageUuid
	}

	doc, err := xmlpath.Parse(strings.NewReader(content.BodyXML))
	if err != nil {
		log.Printf("Unable to parse document", err)
		return nil, err
	}

	matches := xpath.Iter(doc)
	for {
		if !matches.Next() {
			break
		}

		n := matches.Node()
		auxImageUuid, found := util.ExtractUuid(n.String())
		if found {
			imageSets[auxImageUuid] = auxImageUuid
		}
	}

	var imageSetUuids []string
	for uuid, _ := range imageSets {
		imageSetUuids = append(imageSetUuids, uuid)
	}

	return imageSetUuids, nil
}

func (c checker) checkImageModel(uuid string, imageSetUuid string, imageModelUuid string) []string {
	imageModel, err := c.fetchContent(imageModelUuid)
	if err != nil {
		log.Printf("Error retrieving image model: %s", err.Error())
		return []string{uuid, imageSetUuid, imageModelUuid}
	}

	imageBinaryUrl := imageModel.BinaryUrl
	req, _ := http.NewRequest("GET", imageBinaryUrl, nil)

	resp, err := c.client.Do(req)

	if err != nil {
		log.Printf("Error retrieving binary: %s", err.Error())
		return []string{uuid, imageSetUuid, imageModelUuid, imageBinaryUrl}
	}

	if resp.StatusCode != http.StatusOK {
		return []string{uuid, imageSetUuid, imageModelUuid, imageBinaryUrl}
	}

	return []string{}
}
